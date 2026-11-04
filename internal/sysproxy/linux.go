package sysproxy

import (
	"fmt"
	"strconv"
	"strings"
)

// KDEState: original values of [Proxy Settings] in kioslaverc ("" = key absent).
type KDEState struct {
	ProxyType  string `json:"proxyType"`
	SocksProxy string `json:"socksProxy"`
	NoProxyFor string `json:"noProxyFor"`
}

// GNOMEState: original gsettings values (raw gsettings syntax, e.g. 'none', 1080, ['localhost']).
type GNOMEState struct {
	Mode        string `json:"mode"`
	SocksHost   string `json:"socksHost"`
	SocksPort   string `json:"socksPort"`
	IgnoreHosts string `json:"ignoreHosts"`
}

const kdeGroup = "Proxy Settings"

func kdeAvailable() bool {
	_, e1 := lookPath("kwriteconfig6")
	_, e2 := lookPath("kreadconfig6")
	return e1 == nil && e2 == nil
}

func kdeRead(key string) string {
	out, err := run("kreadconfig6", "--file", "kioslaverc", "--group", kdeGroup, "--key", key)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func kdeWrite(key, val string) error {
	if val == "" {
		_, err := run("kwriteconfig6", "--file", "kioslaverc", "--group", kdeGroup, "--key", key, "--delete")
		return err
	}
	_, err := run("kwriteconfig6", "--file", "kioslaverc", "--group", kdeGroup, "--key", key, val)
	return err
}

func kdeNotify() {
	// tell running KDE apps to re-read proxy settings (best effort)
	_, _ = run("dbus-send", "--type=signal", "/KIO/Scheduler", "org.kde.KIO.Scheduler.reparseSlaveConfiguration", "string:")
}

func kdeCapture() *KDEState {
	if !kdeAvailable() {
		return nil
	}
	return &KDEState{ProxyType: kdeRead("ProxyType"), SocksProxy: kdeRead("socksProxy"), NoProxyFor: kdeRead("NoProxyFor")}
}

func kdeApply(port int) error {
	if err := kdeWrite("socksProxy", "socks://127.0.0.1 "+strconv.Itoa(port)); err != nil {
		return err
	}
	if err := kdeWrite("NoProxyFor", strings.Join(bypassLinux(), ",")); err != nil {
		return err
	}
	if err := kdeWrite("ProxyType", "1"); err != nil {
		return err
	}
	kdeNotify()
	return nil
}

func kdeRestore(s *KDEState) error {
	err1 := kdeWrite("ProxyType", s.ProxyType)
	err2 := kdeWrite("socksProxy", s.SocksProxy)
	err3 := kdeWrite("NoProxyFor", s.NoProxyFor)
	kdeNotify()
	for _, e := range []error{err1, err2, err3} {
		if e != nil {
			return e
		}
	}
	return nil
}

func gnomeAvailable() bool {
	if _, err := lookPath("gsettings"); err != nil {
		return false
	}
	_, err := run("gsettings", "get", "org.gnome.system.proxy", "mode")
	return err == nil
}

func gget(schema, key string) string {
	out, _ := run("gsettings", "get", schema, key)
	return strings.TrimSpace(out)
}

func gset(schema, key, val string) error {
	_, err := run("gsettings", "set", schema, key, val)
	return err
}

func gnomeCapture() *GNOMEState {
	if !gnomeAvailable() {
		return nil
	}
	return &GNOMEState{
		Mode:        gget("org.gnome.system.proxy", "mode"),
		SocksHost:   gget("org.gnome.system.proxy.socks", "host"),
		SocksPort:   gget("org.gnome.system.proxy.socks", "port"),
		IgnoreHosts: gget("org.gnome.system.proxy", "ignore-hosts"),
	}
}

func gvariantList(xs []string) string {
	q := make([]string, len(xs))
	for i, x := range xs {
		q[i] = "'" + strings.ReplaceAll(x, "'", "") + "'"
	}
	return "[" + strings.Join(q, ", ") + "]"
}

func gnomeApply(port int) error {
	if err := gset("org.gnome.system.proxy.socks", "host", "'127.0.0.1'"); err != nil {
		return err
	}
	if err := gset("org.gnome.system.proxy.socks", "port", strconv.Itoa(port)); err != nil {
		return err
	}
	if err := gset("org.gnome.system.proxy", "ignore-hosts", gvariantList(bypassLinux())); err != nil {
		return err
	}
	return gset("org.gnome.system.proxy", "mode", "'manual'")
}

func gnomeRestore(s *GNOMEState) error {
	var firstErr error
	for _, kv := range [][3]string{
		{"org.gnome.system.proxy", "mode", s.Mode},
		{"org.gnome.system.proxy.socks", "host", s.SocksHost},
		{"org.gnome.system.proxy.socks", "port", s.SocksPort},
		{"org.gnome.system.proxy", "ignore-hosts", s.IgnoreHosts},
	} {
		if kv[2] == "" {
			continue
		}
		if err := gset(kv[0], kv[1], kv[2]); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("%s %s: %w", kv[0], kv[1], err)
		}
	}
	return firstErr
}

// bypassLinux converts the bypass list to forms KDE/GNOME understand (no "169.254/16" shorthand).
func bypassLinux() []string {
	var out []string
	for _, b := range bypass {
		if b == "169.254/16" {
			b = "169.254.0.0/16"
		}
		out = append(out, b)
	}
	return out
}
