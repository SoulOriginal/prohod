// Package logx is a small leveled logger: in-memory ring for the UI plus a size-capped 0600 file.
package logx

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Entry struct {
	Time  time.Time `json:"time"`
	Level string    `json:"level"`
	Msg   string    `json:"msg"`
}

type Logger struct {
	mu      sync.Mutex
	ring    []Entry
	next    int
	full    bool
	path    string
	f       *os.File
	written int64
	maxFile int64
}

// New opens <dir>/prohod.log (rotating to .1 at 2 MiB). dir == "" keeps logs in memory only.
func New(dir string, ringSize int) *Logger {
	l := &Logger{ring: make([]Entry, ringSize), maxFile: 2 << 20}
	if dir != "" {
		if err := os.MkdirAll(dir, 0o700); err == nil {
			l.path = filepath.Join(dir, "prohod.log")
			l.openFile()
		}
	}
	return l
}

func (l *Logger) openFile() {
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	if st, err := f.Stat(); err == nil {
		l.written = st.Size()
	}
	l.f = f
}

func (l *Logger) add(level, format string, args ...any) {
	e := Entry{Time: time.Now(), Level: level, Msg: fmt.Sprintf(format, args...)}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ring[l.next] = e
	l.next = (l.next + 1) % len(l.ring)
	if l.next == 0 {
		l.full = true
	}
	if l.f != nil {
		line := fmt.Sprintf("%s %-5s %s\n", e.Time.Format("2006-01-02 15:04:05.000"), level, e.Msg)
		n, _ := l.f.WriteString(line)
		l.written += int64(n)
		if l.written > l.maxFile {
			l.f.Close()
			_ = os.Rename(l.path, l.path+".1")
			l.written = 0
			l.openFile()
		}
	}
}

func (l *Logger) Infof(format string, args ...any)  { l.add("INFO", format, args...) }
func (l *Logger) Warnf(format string, args ...any)  { l.add("WARN", format, args...) }
func (l *Logger) Errorf(format string, args ...any) { l.add("ERROR", format, args...) }

// Recent returns up to n newest entries, oldest first.
func (l *Logger) Recent(n int) []Entry {
	l.mu.Lock()
	defer l.mu.Unlock()
	var all []Entry
	if l.full {
		all = append(all, l.ring[l.next:]...)
	}
	all = append(all, l.ring[:l.next]...)
	if n > 0 && len(all) > n {
		all = all[len(all)-n:]
	}
	return all
}

func (l *Logger) Path() string { return l.path }

func (l *Logger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f != nil {
		l.f.Close()
		l.f = nil
	}
}
