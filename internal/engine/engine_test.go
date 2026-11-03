package engine_test

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"prohod/internal/engine"
	"prohod/internal/front"
	"prohod/internal/profile"
)

// The real embedded engine must accept the full generated profile and relay traffic.
func TestRealEngineWithFullProfile(t *testing.T) {
	dir := t.TempDir()
	path, err := engine.Prepare(filepath.Join(dir, "core"))
	if err == engine.ErrUnsupported {
		t.Skip("no engine for this platform")
	}
	if err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o700 {
		t.Fatalf("engine perms %v", fi.Mode().Perm())
	}
	args, err := profile.Build(filepath.Join(dir, "lists"), profile.Options{})
	if err != nil {
		t.Fatal(err)
	}
	p, err := engine.Start(path, args)
	if err != nil {
		t.Fatalf("engine rejected the profile: %v", err)
	}
	defer p.Stop()

	echo, _ := net.Listen("tcp", "127.0.0.1:0")
	defer echo.Close()
	go func() {
		for {
			c, err := echo.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); _, _ = io.Copy(c, c) }()
		}
	}()
	c, err := front.DialVia(p.Port(), "127.0.0.1", echo.Addr().(*net.TCPAddr).Port, 3*time.Second)
	if err != nil {
		t.Fatalf("dial through engine: %v", err)
	}
	_, _ = c.Write([]byte("ping"))
	buf := make([]byte, 4)
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.ReadFull(c, buf); err != nil || string(buf) != "ping" {
		t.Fatalf("echo through engine: %q %v", buf, err)
	}
	c.Close()

	start := time.Now()
	p.Stop()
	select {
	case <-p.Done():
	default:
		t.Fatal("engine still running after Stop")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("stop took %v", time.Since(start))
	}
}

func TestTamperedEngineRefused(t *testing.T) {
	dir := t.TempDir()
	path, err := engine.Prepare(filepath.Join(dir, "core"))
	if err != nil {
		t.Skip(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho pwned\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Start(path, nil); err == nil {
		t.Fatal("tampered engine must not start")
	}
}
