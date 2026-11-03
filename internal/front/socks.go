// Package front is the loopback SOCKS5 server the system proxy points at. It accepts CONNECT requests,
// opens the same request through the engine and relays bytes while counting them per service.
package front

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	"prohod/internal/stats"
)

const (
	maxConns       = 1024
	handshakeLimit = 10 * time.Second
)

type Server struct {
	ln         net.Listener
	enginePort func() int
	classify   func(host string) string
	st         *stats.Stats
	sem        chan struct{}
	wg         sync.WaitGroup
	mu         sync.Mutex
	conns      map[net.Conn]struct{}
	closed     bool
}

// Listen starts the server on 127.0.0.1:<random>.
func Listen(enginePort func() int, classify func(string) string, st *stats.Stats) (*Server, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &Server{ln: ln, enginePort: enginePort, classify: classify, st: st,
		sem: make(chan struct{}, maxConns), conns: map[net.Conn]struct{}{}}
	s.wg.Add(1)
	go s.accept()
	return s, nil
}

func (s *Server) Port() int { return s.ln.Addr().(*net.TCPAddr).Port }

func (s *Server) Close() {
	s.mu.Lock()
	s.closed = true
	for c := range s.conns {
		c.Close()
	}
	s.mu.Unlock()
	s.ln.Close()
	s.wg.Wait()
}

func (s *Server) track(c net.Conn, add bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if add {
		if s.closed {
			return false
		}
		s.conns[c] = struct{}{}
	} else {
		delete(s.conns, c)
	}
	return true
}

func (s *Server) accept() {
	defer s.wg.Done()
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return
		}
		// loopback only, but double-check the peer
		if ra, ok := c.RemoteAddr().(*net.TCPAddr); !ok || !ra.IP.IsLoopback() {
			c.Close()
			continue
		}
		select {
		case s.sem <- struct{}{}:
		default:
			c.Close()
			continue
		}
		if !s.track(c, true) {
			c.Close()
			<-s.sem
			continue
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer func() { <-s.sem }()
			defer s.track(c, false)
			defer c.Close()
			s.handle(c)
		}()
	}
}

func (s *Server) handle(c net.Conn) {
	_ = c.SetDeadline(time.Now().Add(handshakeLimit))
	host, port, err := serverHandshake(c)
	if err != nil {
		return
	}
	conn := s.st.Open(s.classify(host))
	up, err := DialVia(s.enginePort(), host, port, 15*time.Second)
	if err != nil {
		_ = reply(c, 0x05) // connection refused
		conn.Close(true)
		return
	}
	if !s.track(up, true) {
		up.Close()
		conn.Close(true)
		return
	}
	defer s.track(up, false)
	defer up.Close()
	if err := reply(c, 0x00); err != nil {
		conn.Close(true)
		return
	}
	_ = c.SetDeadline(time.Time{})
	relay(c, up, conn)
	conn.Close(false)
}

// relay copies both ways until either side closes.
func relay(client, up net.Conn, conn *stats.Conn) {
	done := make(chan struct{}, 2)
	go func() { copyCount(up, client, conn.AddOut); halfClose(up); done <- struct{}{} }()
	go func() { copyCount(client, up, conn.AddIn); halfClose(client); done <- struct{}{} }()
	<-done
	<-done
}

func halfClose(c net.Conn) {
	if tc, ok := c.(*net.TCPConn); ok {
		_ = tc.CloseWrite()
	}
}

func copyCount(dst, src net.Conn, add func(int)) {
	buf := make([]byte, 32<<10)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			add(n)
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// serverHandshake implements the SOCKS5 server side (no auth, CONNECT only).
func serverHandshake(c net.Conn) (string, int, error) {
	var h [2]byte
	if _, err := io.ReadFull(c, h[:]); err != nil {
		return "", 0, err
	}
	if h[0] != 5 || h[1] == 0 {
		return "", 0, errors.New("not socks5")
	}
	methods := make([]byte, h[1])
	if _, err := io.ReadFull(c, methods); err != nil {
		return "", 0, err
	}
	noAuth := false
	for _, m := range methods {
		if m == 0 {
			noAuth = true
		}
	}
	if !noAuth {
		_, _ = c.Write([]byte{5, 0xff})
		return "", 0, errors.New("no acceptable auth")
	}
	if _, err := c.Write([]byte{5, 0}); err != nil {
		return "", 0, err
	}
	var req [4]byte
	if _, err := io.ReadFull(c, req[:]); err != nil {
		return "", 0, err
	}
	if req[0] != 5 {
		return "", 0, errors.New("bad version")
	}
	if req[1] != 1 { // CONNECT only
		_ = reply(c, 0x07)
		return "", 0, errors.New("command not supported")
	}
	host, err := readAddr(c, req[3])
	if err != nil {
		_ = reply(c, 0x08)
		return "", 0, err
	}
	var p [2]byte
	if _, err := io.ReadFull(c, p[:]); err != nil {
		return "", 0, err
	}
	return host, int(binary.BigEndian.Uint16(p[:])), nil
}

func readAddr(r io.Reader, atyp byte) (string, error) {
	switch atyp {
	case 1:
		var b [4]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return "", err
		}
		return net.IP(b[:]).String(), nil
	case 4:
		var b [16]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return "", err
		}
		return net.IP(b[:]).String(), nil
	case 3:
		var l [1]byte
		if _, err := io.ReadFull(r, l[:]); err != nil {
			return "", err
		}
		b := make([]byte, l[0])
		if _, err := io.ReadFull(r, b); err != nil {
			return "", err
		}
		return string(b), nil
	}
	return "", fmt.Errorf("bad address type %d", atyp)
}

func reply(c net.Conn, code byte) error {
	_, err := c.Write([]byte{5, code, 0, 1, 0, 0, 0, 0, 0, 0})
	return err
}

// DialVia opens host:port through a SOCKS5 proxy on 127.0.0.1:proxyPort (domain names are resolved by the proxy).
func DialVia(proxyPort int, host string, port int, timeout time.Duration) (net.Conn, error) {
	if proxyPort == 0 {
		return nil, errors.New("engine not running")
	}
	c, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(proxyPort)), 3*time.Second)
	if err != nil {
		return nil, err
	}
	_ = c.SetDeadline(time.Now().Add(timeout))
	fail := func(e error) (net.Conn, error) { c.Close(); return nil, e }
	if _, err := c.Write([]byte{5, 1, 0}); err != nil {
		return fail(err)
	}
	var r [2]byte
	if _, err := io.ReadFull(c, r[:]); err != nil {
		return fail(err)
	}
	if r[0] != 5 || r[1] != 0 {
		return fail(errors.New("proxy auth refused"))
	}
	req := []byte{5, 1, 0}
	if ip := net.ParseIP(host); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			req = append(append(req, 1), v4...)
		} else {
			req = append(append(req, 4), ip.To16()...)
		}
	} else {
		if len(host) > 255 {
			return fail(errors.New("host too long"))
		}
		req = append(append(req, 3, byte(len(host))), host...)
	}
	req = binary.BigEndian.AppendUint16(req, uint16(port))
	if _, err := c.Write(req); err != nil {
		return fail(err)
	}
	var rh [4]byte
	if _, err := io.ReadFull(c, rh[:]); err != nil {
		return fail(err)
	}
	if rh[1] != 0 {
		return fail(fmt.Errorf("proxy connect failed (%d)", rh[1]))
	}
	if _, err := readAddr(c, rh[3]); err != nil {
		return fail(err)
	}
	var p [2]byte
	if _, err := io.ReadFull(c, p[:]); err != nil {
		return fail(err)
	}
	_ = c.SetDeadline(time.Time{})
	return c, nil
}
