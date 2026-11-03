// Package engine runs the embedded traffic-shaping engine: extract (0700, sha256-verified before every start),
// start on 127.0.0.1:<random>, wait until it listens, stop reliably.
package engine

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// embedded is set per platform (engine_<os>_<arch>.go). Empty on unsupported platforms.
var embedded []byte

var ErrUnsupported = errors.New("engine is not available for this platform")

func digest() string {
	s := sha256.Sum256(embedded)
	return hex.EncodeToString(s[:])
}

// Prepare extracts the engine into dir (if missing or modified) and returns its path.
func Prepare(dir string) (string, error) {
	if len(embedded) == 0 {
		return "", ErrUnsupported
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	_ = os.Chmod(dir, 0o700)
	want := digest()
	path := filepath.Join(dir, "core-"+want[:12])
	if verify(path, want) == nil {
		return path, nil
	}
	tmp, err := os.CreateTemp(dir, ".core-*")
	if err != nil {
		return "", err
	}
	if _, err := tmp.Write(embedded); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", err
	}
	tmp.Close()
	if err := os.Chmod(tmp.Name(), 0o700); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return path, verify(path, want)
}

func verify(path, want string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	s := sha256.Sum256(b)
	if hex.EncodeToString(s[:]) != want {
		return fmt.Errorf("engine integrity check failed")
	}
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	if st.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("engine file permissions too open")
	}
	return nil
}

// Proc is a running engine.
type Proc struct {
	cmd    *exec.Cmd
	port   int
	done   chan struct{}
	err    error
	stderr bytes.Buffer
	once   sync.Once
}

func (p *Proc) Port() int             { return p.port }
func (p *Proc) Pid() int              { return p.cmd.Process.Pid }
func (p *Proc) Done() <-chan struct{} { return p.done }
func (p *Proc) Err() error            { return p.err }

// Start launches the engine with args on a free loopback port and waits (up to 3 s) until it accepts.
func Start(path string, args []string) (*Proc, error) {
	if err := verify(path, digest()); err != nil {
		return nil, err
	}
	port, err := freePort()
	if err != nil {
		return nil, err
	}
	full := append([]string{"-i", "127.0.0.1", "-p", strconv.Itoa(port)}, args...)
	cmd := exec.Command(path, full...)
	cmd.Env = []string{} // nothing inherited
	cmd.SysProcAttr = sysProcAttr()
	p := &Proc{cmd: cmd, port: port, done: make(chan struct{})}
	cmd.Stderr = &limited{b: &p.stderr, max: 8 << 10}
	cmd.Stdout = cmd.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	go func() { p.err = cmd.Wait(); close(p.done) }()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-p.done:
			return nil, fmt.Errorf("engine exited at start")
		default:
		}
		c, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 200*time.Millisecond)
		if err == nil {
			c.Close()
			return p, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	p.Stop()
	return nil, fmt.Errorf("engine did not start listening")
}

// Stop terminates the engine: SIGTERM, then SIGKILL after 1 s (the engine ignores SIGTERM when idle on macOS).
func (p *Proc) Stop() {
	p.once.Do(func() {
		_ = p.cmd.Process.Signal(syscall.SIGTERM)
		select {
		case <-p.done:
			return
		case <-time.After(time.Second):
		}
		_ = p.cmd.Process.Kill()
		<-p.done
	})
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

type limited struct {
	mu  sync.Mutex
	b   *bytes.Buffer
	max int
}

func (l *limited) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if room := l.max - l.b.Len(); room > 0 {
		if len(p) > room {
			l.b.Write(p[:room])
		} else {
			l.b.Write(p)
		}
	}
	return len(p), nil
}
