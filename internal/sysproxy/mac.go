package sysproxy

import (
	"fmt"
	"strconv"
	"strings"
)

// MacSvc is the original SOCKS + bypass configuration of one macOS network service.
type MacSvc struct {
	Name    string   `json:"name"`
	Enabled bool     `json:"enabled"`
	Server  string   `json:"server"`
	Port    int      `json:"port"`
	Bypass  []string `json:"bypass"`
}

// parseServices parses `networksetup -listallnetworkservices` (skips header and disabled "*" services).
func parseServices(out string) []string {
	var svcs []string
	for i, l := range strings.Split(out, "\n") {
		l = strings.TrimSpace(l)
		if i == 0 && strings.Contains(l, "asterisk") {
			continue
		}
		if l == "" || strings.HasPrefix(l, "*") || strings.Contains(l, "denotes that a network service is disabled") {
			continue
		}
		svcs = append(svcs, l)
	}
	return svcs
}

// parseSocks parses `networksetup -getsocksfirewallproxy <svc>`.
func parseSocks(out string) (enabled bool, server string, port int) {
	for _, l := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(l, ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch strings.TrimSpace(k) {
		case "Enabled":
			enabled = v == "Yes"
		case "Server":
			server = v
		case "Port":
			port, _ = strconv.Atoi(v)
		}
	}
	return
}

// parseBypass parses `networksetup -getproxybypassdomains <svc>`.
func parseBypass(out string) []string {
	if strings.Contains(out, "There aren't any bypass domains") {
		return nil
	}
	var list []string
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			list = append(list, l)
		}
	}
	return list
}

func macCapture() ([]MacSvc, error) {
	out, err := run("networksetup", "-listallnetworkservices")
	if err != nil {
		return nil, fmt.Errorf("network services: %w", err)
	}
	var res []MacSvc
	for _, name := range parseServices(out) {
		so, err := run("networksetup", "-getsocksfirewallproxy", name)
		if err != nil {
			continue // service without proxy support
		}
		en, srv, port := parseSocks(so)
		bo, _ := run("networksetup", "-getproxybypassdomains", name)
		res = append(res, MacSvc{Name: name, Enabled: en, Server: srv, Port: port, Bypass: parseBypass(bo)})
	}
	if len(res) == 0 {
		return nil, fmt.Errorf("no network services found")
	}
	return res, nil
}

func merge(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range append(append([]string{}, a...), b...) {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

func macApply(svcs []MacSvc, port int) error {
	var firstErr error
	for _, s := range svcs {
		args := append([]string{"-setproxybypassdomains", s.Name}, merge(s.Bypass, bypass)...)
		if _, err := run("networksetup", args...); err != nil && firstErr == nil {
			firstErr = err
		}
		if _, err := run("networksetup", "-setsocksfirewallproxy", s.Name, "127.0.0.1", strconv.Itoa(port)); err != nil && firstErr == nil {
			firstErr = err
		}
		if _, err := run("networksetup", "-setsocksfirewallproxystate", s.Name, "on"); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func macRestore(svcs []MacSvc) error {
	var firstErr error
	for _, s := range svcs {
		if s.Server != "" && s.Port > 0 {
			if _, err := run("networksetup", "-setsocksfirewallproxy", s.Name, s.Server, strconv.Itoa(s.Port)); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		state := "off"
		if s.Enabled {
			state = "on"
		}
		if _, err := run("networksetup", "-setsocksfirewallproxystate", s.Name, state); err != nil && firstErr == nil {
			firstErr = err
		}
		bp := s.Bypass
		if len(bp) == 0 {
			bp = []string{"Empty"}
		}
		if _, err := run("networksetup", append([]string{"-setproxybypassdomains", s.Name}, bp...)...); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
