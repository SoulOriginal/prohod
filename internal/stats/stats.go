// Package stats keeps traffic counters: totals, speed and per-service health. Safe for concurrent use.
package stats

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type Service struct {
	Name     string `json:"name"`
	Active   int64  `json:"active"`
	OK       int64  `json:"ok"`
	Failed   int64  `json:"failed"`
	BytesIn  int64  `json:"bytesIn"`
	BytesOut int64  `json:"bytesOut"`
	LastOK   int64  `json:"lastOk"`   // unix ms
	LastFail int64  `json:"lastFail"` // unix ms
}

type Snapshot struct {
	BytesIn     int64     `json:"bytesIn"`
	BytesOut    int64     `json:"bytesOut"`
	SpeedIn     int64     `json:"speedIn"`  // bytes/s, ~2 s window
	SpeedOut    int64     `json:"speedOut"` // bytes/s
	Connections int64     `json:"connections"`
	Total       int64     `json:"total"`
	Failed      int64     `json:"failed"`
	Services    []Service `json:"services"`
}

type Stats struct {
	in, out, active, total, failed atomic.Int64

	mu       sync.Mutex
	services map[string]*Service
	order    []string
	samples  []sample
}

type sample struct {
	t       time.Time
	in, out int64
}

func New(serviceOrder []string) *Stats {
	s := &Stats{services: map[string]*Service{}, order: serviceOrder}
	for _, n := range serviceOrder {
		s.services[n] = &Service{Name: n}
	}
	return s
}

// Conn tracks one proxied connection.
type Conn struct {
	s       *Stats
	svc     *Service
	in, out atomic.Int64
	ended   atomic.Bool
}

// Open registers a connection to a (possibly empty) service name.
func (s *Stats) Open(service string) *Conn {
	s.active.Add(1)
	s.total.Add(1)
	c := &Conn{s: s}
	if service != "" {
		s.mu.Lock()
		svc := s.services[service]
		if svc == nil {
			svc = &Service{Name: service}
			s.services[service] = svc
			s.order = append(s.order, service)
		}
		svc.Active++
		c.svc = svc
		s.mu.Unlock()
	}
	return c
}

func (c *Conn) AddIn(n int)  { c.in.Add(int64(n)); c.s.in.Add(int64(n)) }
func (c *Conn) AddOut(n int) { c.out.Add(int64(n)); c.s.out.Add(int64(n)) }

// Close ends the connection. It counts as failed if nothing came back from the destination.
func (c *Conn) Close(dialFailed bool) {
	if c.ended.Swap(true) {
		return
	}
	c.s.active.Add(-1)
	fail := dialFailed || c.in.Load() == 0
	if fail {
		c.s.failed.Add(1)
	}
	if c.svc == nil {
		return
	}
	now := time.Now().UnixMilli()
	c.s.mu.Lock()
	c.svc.Active--
	c.svc.BytesIn += c.in.Load()
	c.svc.BytesOut += c.out.Load()
	if fail {
		c.svc.Failed++
		c.svc.LastFail = now
	} else {
		c.svc.OK++
		c.svc.LastOK = now
	}
	c.s.mu.Unlock()
}

// Snapshot returns current counters; call it periodically (it also drives the speed window).
func (s *Stats) Snapshot() Snapshot {
	now := time.Now()
	in, out := s.in.Load(), s.out.Load()
	s.mu.Lock()
	s.samples = append(s.samples, sample{now, in, out})
	for len(s.samples) > 1 && now.Sub(s.samples[0].t) > 2500*time.Millisecond {
		s.samples = s.samples[1:]
	}
	var spIn, spOut int64
	if f := s.samples[0]; len(s.samples) > 1 {
		if dt := now.Sub(f.t).Seconds(); dt > 0.2 {
			spIn = int64(float64(in-f.in) / dt)
			spOut = int64(float64(out-f.out) / dt)
		}
	}
	svcs := make([]Service, 0, len(s.order))
	for _, n := range s.order {
		svcs = append(svcs, *s.services[n])
	}
	s.mu.Unlock()
	sort.SliceStable(svcs, func(i, j int) bool { return svcs[i].OK+svcs[i].Failed > svcs[j].OK+svcs[j].Failed })
	return Snapshot{BytesIn: in, BytesOut: out, SpeedIn: spIn, SpeedOut: spOut,
		Connections: s.active.Load(), Total: s.total.Load(), Failed: s.failed.Load(), Services: svcs}
}
