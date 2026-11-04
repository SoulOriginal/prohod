package sysproxy

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const listOut = `An asterisk (*) denotes that a network service is disabled.
Thunderbolt Bridge
Wi-Fi
*Old VPN
Ethernet
`

func TestParsers(t *testing.T) {
	if got := parseServices(listOut); strings.Join(got, "|") != "Thunderbolt Bridge|Wi-Fi|Ethernet" {
		t.Fatalf("services: %v", got)
	}
	en, srv, port := parseSocks("Enabled: Yes\nServer: 10.0.0.5\nPort: 1080\nAuthenticated Proxy Enabled: 0\n")
	if !en || srv != "10.0.0.5" || port != 1080 {
		t.Fatalf("socks: %v %q %d", en, srv, port)
	}
	if b := parseBypass("There aren't any bypass domains set on Wi-Fi.\n"); b != nil {
		t.Fatalf("bypass empty: %v", b)
	}
	if b := parseBypass("*.local\n169.254/16\n"); len(b) != 2 {
		t.Fatalf("bypass: %v", b)
	}
}

// fakeMac simulates networksetup state so capture → apply → restore can be checked end to end.
type fakeMac struct {
	socks  map[string][3]string // enabled, server, port
	bypass map[string][]string
	calls  []string
}

func (f *fakeMac) run(name string, args ...string) (string, error) {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	if name != "networksetup" {
		return "", errors.New("unexpected " + name)
	}
	switch args[0] {
	case "-listallnetworkservices":
		return listOut, nil
	case "-getsocksfirewallproxy":
		s := f.socks[args[1]]
		return "Enabled: " + s[0] + "\nServer: " + s[1] + "\nPort: " + s[2] + "\n", nil
	case "-getproxybypassdomains":
		if len(f.bypass[args[1]]) == 0 {
			return "There aren't any bypass domains set on " + args[1] + ".\n", nil
		}
		return strings.Join(f.bypass[args[1]], "\n") + "\n", nil
	case "-setsocksfirewallproxy":
		s := f.socks[args[1]]
		f.socks[args[1]] = [3]string{"Yes", args[2], args[3]}
		_ = s
	case "-setsocksfirewallproxystate":
		s := f.socks[args[1]]
		if args[2] == "on" {
			s[0] = "Yes"
		} else {
			s[0] = "No"
		}
		f.socks[args[1]] = s
	case "-setproxybypassdomains":
		if len(args) == 3 && args[2] == "Empty" {
			f.bypass[args[1]] = nil
		} else {
			f.bypass[args[1]] = append([]string(nil), args[2:]...)
		}
	}
	return "", nil
}

func TestMacCaptureApplyRestoreRoundTrip(t *testing.T) {
	f := &fakeMac{
		socks: map[string][3]string{
			"Thunderbolt Bridge": {"No", "", "0"},
			"Wi-Fi":              {"No", "", "0"},
			"Ethernet":           {"Yes", "10.1.1.1", "9050"},
		},
		bypass: map[string][]string{"Wi-Fi": {"*.local", "169.254/16"}},
	}
	oldRun, oldOS := run, goos
	run, goos = f.run, "darwin"
	defer func() { run, goos = oldRun, oldOS }()

	orig := map[string][3]string{}
	origBypass := map[string]string{}
	for k, v := range f.socks {
		orig[k] = v
		origBypass[k] = strings.Join(f.bypass[k], ",")
	}

	st, err := Capture(5555)
	if err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(t.TempDir(), "state.json")
	if err := SaveState(statePath, st); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(statePath); fi.Mode().Perm() != 0o600 {
		t.Fatalf("state perms %v", fi.Mode().Perm())
	}
	if err := Apply(st); err != nil {
		t.Fatal(err)
	}
	if s := f.socks["Wi-Fi"]; s != [3]string{"Yes", "127.0.0.1", "5555"} {
		t.Fatalf("applied: %v", s)
	}
	if !contains(f.bypass["Wi-Fi"], "100.64.0.0/10") || !contains(f.bypass["Wi-Fi"], "*.local") {
		t.Fatalf("bypass after apply: %v", f.bypass["Wi-Fi"])
	}

	// simulate a crash: restore from the file only
	repaired, err := RepairStale(statePath)
	if !repaired || err != nil {
		t.Fatalf("repair: %v %v", repaired, err)
	}
	for k, v := range orig {
		got := f.socks[k]
		if got[0] != v[0] || (v[1] != "" && (got[1] != v[1] || got[2] != v[2])) {
			t.Errorf("%s socks restored to %v, want %v", k, got, v)
		}
		if strings.Join(f.bypass[k], ",") != origBypass[k] {
			t.Errorf("%s bypass restored to %v, want %q", k, f.bypass[k], origBypass[k])
		}
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatal("state file must be removed after repair")
	}
}

func TestRepairStaleNoFile(t *testing.T) {
	repaired, err := RepairStale(filepath.Join(t.TempDir(), "none.json"))
	if repaired || err != nil {
		t.Fatalf("got %v %v", repaired, err)
	}
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
