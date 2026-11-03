package front

import (
	"bytes"
	"io"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"prohod/internal/stats"
)

// fakeEngine is a minimal SOCKS5 server that "connects" to an in-memory echo and records requested hosts.
type fakeEngine struct {
	ln    net.Listener
	mu    sync.Mutex
	hosts []string
	fail  bool
}

func newFakeEngine(t *testing.T) *fakeEngine {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeEngine{ln: ln}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(c)
		}
	}()
	t.Cleanup(func() { ln.Close() })
	return f
}

func (f *fakeEngine) port() int { return f.ln.Addr().(*net.TCPAddr).Port }

func (f *fakeEngine) serve(c net.Conn) {
	defer c.Close()
	host, port, err := serverHandshake(c)
	if err != nil {
		return
	}
	f.mu.Lock()
	f.hosts = append(f.hosts, host+":"+strconv.Itoa(port))
	fail := f.fail
	f.mu.Unlock()
	if fail {
		_ = reply(c, 0x05)
		return
	}
	_ = reply(c, 0)
	_, _ = io.Copy(c, c) // echo
}

func TestRelayCountsAndClassifies(t *testing.T) {
	eng := newFakeEngine(t)
	st := stats.New([]string{"Instagram"})
	classify := func(h string) string {
		if h == "www.instagram.com" {
			return "Instagram"
		}
		return ""
	}
	s, err := Listen(eng.port, classify, st)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	c, err := DialVia(s.Port(), "www.instagram.com", 443, 3*time.Second)
	if err != nil {
		t.Fatalf("dial through front: %v", err)
	}
	msg := bytes.Repeat([]byte("x"), 70000)
	go func() { _, _ = c.Write(msg); c.(*net.TCPConn).CloseWrite() }()
	got, err := io.ReadAll(c)
	if err != nil || !bytes.Equal(got, msg) {
		t.Fatalf("echo mismatch: %d bytes, err %v", len(got), err)
	}
	c.Close()
	time.Sleep(100 * time.Millisecond)

	snap := st.Snapshot()
	if snap.BytesIn != 70000 || snap.BytesOut != 70000 {
		t.Fatalf("bytes in/out = %d/%d", snap.BytesIn, snap.BytesOut)
	}
	if snap.Services[0].Name != "Instagram" || snap.Services[0].OK != 1 {
		t.Fatalf("service stats: %+v", snap.Services[0])
	}
	if eng.hosts[0] != "www.instagram.com:443" {
		t.Fatalf("engine got %v (domain must be passed, not resolved)", eng.hosts)
	}
}

func TestEngineRefusalCountsAsFailure(t *testing.T) {
	eng := newFakeEngine(t)
	eng.fail = true
	st := stats.New(nil)
	s, err := Listen(eng.port, func(string) string { return "" }, st)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := DialVia(s.Port(), "blocked.example", 443, 2*time.Second); err == nil {
		t.Fatal("expected failure")
	}
	time.Sleep(50 * time.Millisecond)
	if snap := st.Snapshot(); snap.Failed != 1 || snap.Connections != 0 {
		t.Fatalf("failed=%d active=%d", snap.Failed, snap.Connections)
	}
}

func TestRejectsNonConnectAndGarbage(t *testing.T) {
	eng := newFakeEngine(t)
	s, err := Listen(eng.port, func(string) string { return "" }, stats.New(nil))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c, _ := net.Dial("tcp", "127.0.0.1:"+strconv.Itoa(s.Port()))
	defer c.Close()
	_, _ = c.Write([]byte("GET / HTTP/1.1\r\n\r\n"))
	_ = c.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := c.Read(make([]byte, 10)); err == nil {
		t.Fatal("garbage must be dropped")
	}
	// UDP ASSOCIATE (cmd 3) must be refused
	u, _ := net.Dial("tcp", "127.0.0.1:"+strconv.Itoa(s.Port()))
	defer u.Close()
	_, _ = u.Write([]byte{5, 1, 0})
	r := make([]byte, 2)
	_, _ = io.ReadFull(u, r)
	_, _ = u.Write([]byte{5, 3, 0, 1, 127, 0, 0, 1, 0, 53})
	rep := make([]byte, 10)
	_ = u.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := io.ReadFull(u, rep); err != nil || rep[1] != 0x07 {
		t.Fatalf("want command-not-supported, got %v %v", rep, err)
	}
}

func TestCloseDropsActiveConnections(t *testing.T) {
	eng := newFakeEngine(t)
	s, err := Listen(eng.port, func(string) string { return "" }, stats.New(nil))
	if err != nil {
		t.Fatal(err)
	}
	c, err := DialVia(s.Port(), "a.example", 443, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { s.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Close hung with an active relay")
	}
	_ = c.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := c.Read(make([]byte, 1)); err == nil {
		t.Fatal("connection should be closed")
	}
}
