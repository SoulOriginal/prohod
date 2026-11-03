// Package profile holds the routing knowledge: method table, pinned domain lists and the fallback chain,
// and turns them into an engine argument vector. Method internals never leave this package; the rest of the
// app (and the UI) only sees neutral labels.
package profile

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed pins/*.txt
var pinFS embed.FS

// Method is one traffic-shaping recipe. Label is what users may see.
type Method struct {
	ID    string
	Label string
	args  []string
}

var methods = map[string]Method{
	"S01": {"S01", "A", []string{"-r1+s", "-o1"}},
	"S02": {"S02", "B", []string{"-s25", "-r5+s", "-s25+s"}},
	"S03": {"S03", "C", []string{"-o1"}},
	"S04": {"S04", "D", []string{"-o1", "-r-5+se"}},
	"S05": {"S05", "E", []string{"-d1"}},
	"S06": {"S06", "F", []string{"-o3", "-d7"}},
	"S09": {"S09", "G", []string{"-o2", "-s1", "-q1"}},
	"S08": {"S08", "H", []string{"-d1", "-s1+s", "-d3+s", "-s6+s", "-d9+s", "-s12+s", "-d15+s", "-s20+s", "-d25+s", "-s30+s", "-d35+s"}},
	"S10": {"S10", "I", []string{"-r1+s", "-d1"}},
}

// Order of pinned groups (first match wins) and of the automatic fallback chain.
// Chain order comes from greedy set cover over a field test on a real censoring ISP.
var (
	pinOrder   = []string{"S01", "S04", "S02", "S03", "S05", "S06", "S09", "S08", "S10"}
	chainOrder = []string{"S02", "S03", "S05", "S01", "S06", "S09"}
)

// Options tune the engine; zero value = defaults.
type Options struct {
	TLSTimeoutSec float64 // no ServerHello within this time -> try next method (default 2)
	CacheTTLSec   int     // how long a working method stays bound to a destination IP (default 86400)
	IPv4Only      bool
}

// Pins returns method ID -> domains (deduplicated, sorted).
func Pins() map[string][]string {
	out := map[string][]string{}
	for _, id := range pinOrder {
		b, err := pinFS.ReadFile("pins/" + id + ".txt")
		if err != nil {
			continue
		}
		seen := map[string]bool{}
		for _, l := range strings.Split(string(b), "\n") {
			l = strings.ToLower(strings.TrimSpace(l))
			if l == "" || strings.HasPrefix(l, "#") || seen[l] {
				continue
			}
			seen[l] = true
			out[id] = append(out[id], l)
		}
		sort.Strings(out[id])
	}
	return out
}

// Build writes the domain lists into listDir (0700/0600) and returns the engine arguments (without the
// listen address/port, which the engine package adds).
func Build(listDir string, o Options) ([]string, error) {
	if o.TLSTimeoutSec <= 0 {
		o.TLSTimeoutSec = 2
	}
	if o.CacheTTLSec <= 0 {
		o.CacheTTLSec = 86400
	}
	if err := os.MkdirAll(listDir, 0o700); err != nil {
		return nil, err
	}
	args := []string{"-W", fmt.Sprintf("%g", o.TLSTimeoutSec), "-u", fmt.Sprint(o.CacheTTLSec)}
	if o.IPv4Only {
		args = append(args, "-X")
	}
	pins := Pins()
	first := true
	for _, id := range pinOrder {
		doms := pins[id]
		if len(doms) == 0 {
			continue
		}
		f := filepath.Join(listDir, "l"+id[1:]+".txt")
		if err := os.WriteFile(f, []byte(strings.Join(doms, "\n")+"\n"), 0o600); err != nil {
			return nil, err
		}
		if !first {
			args = append(args, "-An") // reached when the previous group was skipped (host not in its list)
		}
		args = append(args, "-H", f)
		args = append(args, methods[id].args...)
		first = false
	}
	args = append(args, "-An") // everything else: untouched first
	for _, id := range chainOrder {
		args = append(args, "-A", "t,s") // timeout/reset or no ServerHello -> next method, result cached
		args = append(args, methods[id].args...)
	}
	return args, nil
}

// ServiceOf maps a hostname to a user-facing service name ("" = not a tracked service).
func ServiceOf(host string) string {
	h := strings.ToLower(strings.TrimSuffix(host, "."))
	for _, s := range services {
		for _, suf := range s.domains {
			if h == suf || strings.HasSuffix(h, "."+suf) {
				return s.name
			}
		}
	}
	return ""
}

// Services lists the tracked services in display order.
func Services() []string {
	out := make([]string, 0, len(services))
	for _, s := range services {
		out = append(out, s.name)
	}
	return out
}

var services = []struct {
	name    string
	domains []string
}{
	{"Instagram", []string{"instagram.com", "cdninstagram.com"}},
	{"YouTube", []string{"youtube.com", "youtu.be", "googlevideo.com", "ytimg.com", "ggpht.com", "youtube-nocookie.com", "youtubei.googleapis.com"}},
	{"Facebook", []string{"facebook.com", "fbcdn.net", "fb.com", "fb.me", "fbsbx.com", "facebook.net", "messenger.com"}},
	{"Threads", []string{"threads.com", "threads.net"}},
	{"WhatsApp", []string{"whatsapp.com", "whatsapp.net"}},
	{"X", []string{"x.com", "twitter.com", "twimg.com", "t.co"}},
	{"Discord", []string{"discord.com", "discord.gg", "discordapp.com", "discordapp.net", "discord.media"}},
	{"LinkedIn", []string{"linkedin.com", "licdn.com"}},
	{"SoundCloud", []string{"soundcloud.com", "sndcdn.com"}},
	{"Patreon", []string{"patreon.com", "patreonusercontent.com"}},
	{"Signal", []string{"signal.org", "whispersystems.org"}},
	{"Viber", []string{"viber.com"}},
	{"Rutracker", []string{"rutracker.org", "rutracker.cc"}},
}
