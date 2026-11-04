// Package core is the app controller: it wires engine, front proxy, system proxy, guard, stats and logs.
// Everything user-facing goes through here; messages are deliberately neutral (no method internals).
package core

import (
	"crypto/tls"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"prohod/internal/engine"
	"prohod/internal/front"
	"prohod/internal/guard"
	"prohod/internal/logx"
	"prohod/internal/profile"
	"prohod/internal/stats"
	"prohod/internal/sysproxy"
)

const (
	StateOff        = "off"
	StateConnecting = "connecting"
	StateOn         = "on"
	StateStopping   = "stopping"
)

type Check struct {
	Name string `json:"name"`
	OK   bool   `json:"ok"`
	Done bool   `json:"done"`
	Ms   int64  `json:"ms"`
}

type Status struct {
	State   string  `json:"state"`
	Since   int64   `json:"since"` // unix ms of entering State
	Message string  `json:"message"`
	Pinned  int     `json:"pinned"`
	Checks  []Check `json:"checks"`
}

type Settings struct {
	IPv4Only    bool `json:"ipv4Only"`
	AutoConnect bool `json:"autoConnect"`
}

type Controller struct {
	mu        sync.Mutex
	status    Status
	cfgDir    string
	cacheDir  string
	statePath string
	settings  Settings

	Log   *logx.Logger
	Stats *stats.Stats

	proc     *engine.Proc
	front    *front.Server
	proxySt  *sysproxy.State
	restarts []time.Time
	gen      int // connection generation, guards stale goroutines
}

// SelfTestTargets are probed after connecting (TLS handshake through our own proxy).
var SelfTestTargets = []struct{ Name, Host string }{
	{"Интернет", "www.google.com"},
	{"Instagram", "www.instagram.com"},
	{"YouTube", "www.youtube.com"},
}

func New() (*Controller, error) {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	c := &Controller{
		cfgDir:   filepath.Join(cfg, "prohod"),
		cacheDir: filepath.Join(cache, "prohod"),
		Stats:    stats.New(profile.Services()),
	}
	c.statePath = filepath.Join(c.cfgDir, "proxy-state.json")
	if err := os.MkdirAll(c.cfgDir, 0o700); err != nil {
		return nil, err
	}
	c.Log = logx.New(filepath.Join(c.cfgDir, "logs"), 500)
	c.loadSettings()
	c.status = Status{State: StateOff, Since: now(), Pinned: countPinned()}
	if repaired, err := sysproxy.RepairStale(c.statePath); repaired {
		if err != nil {
			c.Log.Warnf("Восстановление сетевых настроек после прошлого сбоя: частично (%v)", err)
		} else {
			c.Log.Infof("Сетевые настройки восстановлены после прошлого незавершённого сеанса")
		}
	}
	return c, nil
}

func now() int64 { return time.Now().UnixMilli() }

func countPinned() int {
	n := 0
	for _, d := range profile.Pins() {
		n += len(d)
	}
	return n
}

func (c *Controller) setState(s, msg string) {
	c.status.State, c.status.Message, c.status.Since = s, msg, now()
}

func (c *Controller) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	st := c.status
	st.Checks = append([]Check(nil), c.status.Checks...)
	return st
}

func (c *Controller) enginePort() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.proc == nil {
		return 0
	}
	return c.proc.Port()
}

// Connect starts everything. Order matters: engine → front → save state → system proxy → guard.
func (c *Controller) Connect() error {
	c.mu.Lock()
	if c.status.State != StateOff {
		c.mu.Unlock()
		return nil
	}
	c.setState(StateConnecting, "Подключение…")
	c.status.Checks = nil
	c.gen++
	gen := c.gen
	settings := c.settings
	c.mu.Unlock()

	fail := func(err error, userMsg string) error {
		c.Log.Errorf("%s: %v", userMsg, err)
		c.teardown()
		c.mu.Lock()
		c.setState(StateOff, userMsg)
		c.mu.Unlock()
		return errors.New(userMsg)
	}

	path, err := engine.Prepare(filepath.Join(c.cacheDir, "core"))
	if err != nil {
		return fail(err, "Модуль защиты недоступен")
	}
	args, err := profile.Build(filepath.Join(c.cfgDir, "lists"), profile.Options{IPv4Only: settings.IPv4Only || !ipv6Works()})
	if err != nil {
		return fail(err, "Не удалось подготовить профиль")
	}
	proc, err := engine.Start(path, args)
	if err != nil {
		return fail(err, "Не удалось запустить модуль защиты")
	}
	c.mu.Lock()
	c.proc = proc
	c.mu.Unlock()

	fs, err := front.Listen(c.enginePort, profile.ServiceOf, c.Stats)
	if err != nil {
		return fail(err, "Не удалось открыть локальный порт")
	}
	c.mu.Lock()
	c.front = fs
	c.mu.Unlock()

	st, err := sysproxy.Capture(fs.Port())
	if err != nil {
		return fail(err, "Не удалось прочитать сетевые настройки системы")
	}
	st.EnginePID = proc.Pid()
	if err := sysproxy.SaveState(c.statePath, st); err != nil {
		return fail(err, "Не удалось сохранить сетевые настройки")
	}
	c.mu.Lock()
	c.proxySt = st
	c.mu.Unlock()
	if err := sysproxy.Apply(st); err != nil {
		return fail(err, "Не удалось применить сетевые настройки")
	}

	c.mu.Lock()
	c.setState(StateOn, "Защита включена")
	c.mu.Unlock()
	c.Log.Infof("Подключено")
	go c.watchEngine(gen, proc)
	go c.selfTest(gen)
	return nil
}

// watchEngine restarts a crashed engine (max 3 times per minute), otherwise disconnects safely.
func (c *Controller) watchEngine(gen int, p *engine.Proc) {
	<-p.Done()
	c.mu.Lock()
	if c.gen != gen || c.status.State != StateOn || c.proc != p {
		c.mu.Unlock()
		return
	}
	cut := time.Now().Add(-time.Minute)
	recent := c.restarts[:0]
	for _, t := range c.restarts {
		if t.After(cut) {
			recent = append(recent, t)
		}
	}
	c.restarts = recent
	tooMany := len(c.restarts) >= 3
	c.mu.Unlock()

	if !tooMany {
		path, err := engine.Prepare(filepath.Join(c.cacheDir, "core"))
		var args []string
		if err == nil {
			args, err = profile.Build(filepath.Join(c.cfgDir, "lists"), profile.Options{IPv4Only: c.Settings().IPv4Only || !ipv6Works()})
		}
		var np *engine.Proc
		if err == nil {
			np, err = engine.Start(path, args)
		}
		if err == nil {
			c.mu.Lock()
			c.proc = np
			c.restarts = append(c.restarts, time.Now())
			if c.proxySt != nil {
				c.proxySt.EnginePID = np.Pid()
				_ = sysproxy.SaveState(c.statePath, c.proxySt)
			}
			c.mu.Unlock()
			c.Log.Warnf("Модуль защиты перезапущен")
			go c.watchEngine(gen, np)
			return
		}
		c.Log.Errorf("Перезапуск модуля не удался: %v", err)
	}
	c.Log.Errorf("Модуль защиты остановился, соединение отключено")
	_ = c.Disconnect()
	c.mu.Lock()
	c.status.Message = "Отключено из-за сбоя модуля"
	c.mu.Unlock()
}

func (c *Controller) selfTest(gen int) {
	checks := make([]Check, len(SelfTestTargets))
	for i, t := range SelfTestTargets {
		checks[i] = Check{Name: t.Name}
	}
	c.mu.Lock()
	c.status.Checks = checks
	var port int
	if c.front != nil {
		port = c.front.Port()
	}
	c.mu.Unlock()
	var wg sync.WaitGroup
	for i, t := range SelfTestTargets {
		wg.Add(1)
		go func(i int, host string) {
			defer wg.Done()
			start := time.Now()
			ok := tlsProbe(port, host, 12*time.Second)
			c.mu.Lock()
			if c.gen == gen && i < len(c.status.Checks) {
				c.status.Checks[i] = Check{Name: SelfTestTargets[i].Name, OK: ok, Done: true, Ms: time.Since(start).Milliseconds()}
			}
			c.mu.Unlock()
		}(i, t.Host)
	}
	wg.Wait()
	st := c.Status()
	for _, ch := range st.Checks {
		if ch.OK {
			c.Log.Infof("Проверка «%s»: доступно (%d мс)", ch.Name, ch.Ms)
		} else {
			c.Log.Warnf("Проверка «%s»: недоступно", ch.Name)
		}
	}
}

// ipv6Works reports whether this network can reach the internet over IPv6. Without it the engine would try
// IPv6 addresses first and stall ~3 s per connection, so the profile switches to IPv4-only.
func ipv6Works() bool {
	for _, a := range []string{"[2606:4700:4700::1111]:443", "[2001:4860:4860::8888]:443"} {
		if c, err := net.DialTimeout("tcp6", a, 1500*time.Millisecond); err == nil {
			c.Close()
			return true
		}
	}
	return false
}

// tlsProbe completes a TLS handshake to host:443 through our own front proxy.
func tlsProbe(frontPort int, host string, timeout time.Duration) bool {
	conn, err := front.DialVia(frontPort, host, 443, timeout)
	if err != nil {
		return false
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	tc := tls.Client(conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	return tc.Handshake() == nil
}

// teardown restores the system proxy FIRST (never leave the user offline), then stops local pieces.
func (c *Controller) teardown() {
	c.mu.Lock()
	st, fs, p := c.proxySt, c.front, c.proc
	c.proxySt, c.front, c.proc = nil, nil, nil
	c.gen++
	c.mu.Unlock()
	if st != nil {
		if err := sysproxy.Restore(st); err != nil {
			c.Log.Errorf("Восстановление сетевых настроек: %v", err)
		} else {
			_ = os.Remove(c.statePath)
		}
	} else {
		_ = os.Remove(c.statePath)
	}
	if fs != nil {
		fs.Close()
	}
	if p != nil {
		p.Stop()
	}
}

func (c *Controller) Disconnect() error {
	c.mu.Lock()
	if c.status.State == StateOff || c.status.State == StateStopping {
		c.mu.Unlock()
		return nil
	}
	c.setState(StateStopping, "Отключение…")
	c.mu.Unlock()
	c.teardown()
	c.mu.Lock()
	c.setState(StateOff, "Защита выключена")
	c.status.Checks = nil
	c.mu.Unlock()
	c.Log.Infof("Отключено")
	return nil
}

// Shutdown is called on app exit.
func (c *Controller) Shutdown() {
	_ = c.Disconnect()
	c.Log.Close()
}

// StartGuard launches the watchdog process once per app run (it outlives a crash of this process).
func (c *Controller) StartGuard() {
	if err := guard.Spawn(c.statePath); err != nil {
		c.Log.Warnf("Сторож не запущен: %v", err)
	}
}

func (c *Controller) LogDir() string { return filepath.Dir(c.Log.Path()) }
