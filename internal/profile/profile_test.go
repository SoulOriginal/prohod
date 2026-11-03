package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPinsLoaded(t *testing.T) {
	p := Pins()
	if len(p["S01"]) == 0 || !contains(p["S01"], "instagram.com") {
		t.Fatalf("S01 must pin instagram.com, got %v", p["S01"])
	}
	if !contains(p["S04"], "googlevideo.com") {
		t.Fatalf("S04 must pin googlevideo.com")
	}
	for id := range p {
		if _, ok := methods[id]; !ok {
			t.Fatalf("pin list %s has no method", id)
		}
	}
}

func TestBuildStructure(t *testing.T) {
	dir := t.TempDir()
	args, err := Build(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	s := strings.Join(args, " ")
	if !strings.HasPrefix(s, "-W 2 -u 86400 -H ") {
		t.Fatalf("unexpected head: %.60s", s)
	}
	// first group must not be preceded by -An; plain group then chain of -A t,s
	if strings.Count(s, "-A t,s") != len(chainOrder) {
		t.Fatalf("chain size mismatch in %s", s)
	}
	idxPlain := strings.LastIndex(s, "-An -A t,s")
	if idxPlain < 0 {
		t.Fatalf("plain pass-through group missing before chain: %s", s)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.txt"))
	if len(files) != len(Pins()) {
		t.Fatalf("lists written: %d", len(files))
	}
	st, _ := os.Stat(files[0])
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("list perms %v", st.Mode().Perm())
	}
}

func TestServiceOf(t *testing.T) {
	cases := map[string]string{
		"www.instagram.com": "Instagram", "rr3---sn-abc.googlevideo.com": "YouTube",
		"ya.ru": "", "notinstagram.com": "", "x.com": "X",
	}
	for h, want := range cases {
		if got := ServiceOf(h); got != want {
			t.Errorf("ServiceOf(%q)=%q want %q", h, got, want)
		}
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
