package refstate

import (
	"embed"
	"encoding/json"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/xqsit94/cc-statusline/internal/gitinfo"
)

//go:embed payloads/*.json
var files embed.FS

type State struct {
	Name      string
	Reference bool
	Desc      string
	Payload   []byte
	Git       gitinfo.Info
}

var order = []struct {
	name      string
	reference bool
	desc      string
}{
	{"normal-42", true, "42% — everything fine"},
	{"warning-75", true, "75% — one rate limit window"},
	{"danger-92", true, "92% — warn marker and a 1M window"},
	{"startup", true, "clean — every optional segment absent at once"},
	{"long-model", false, "width stress — a 54-character model name"},

	{"null-context", false, "used_percentage null mid-session — the bar reads 0%"},
	{"no-ratelimits", false, "non-subscriber — the ratelimits segment is absent"},
	{"seven-only", false, "seven-day window alone — the untested direction of §5.7"},
	{"no-git", false, "not a repository, but a live session — branch alone is absent"},
	{"detached", false, "detached HEAD — the branch is a short SHA"},
	{"long-branch", false, "a 45-character branch — forces git.branch_max_len"},
	{"500k-context", false, "the third size label — neither 200k nor 1M"},
	{"fractional-pct", false, "p_exact 69.6, p_shown 70 — the one state where §5.3's two percents differ"},
	{"wide-cost", false, "$107.43 with an ordinary model name — a wide cost as the only wide thing"},
	{"sub-minute", false, "59,999ms — one millisecond below §5.3's duration floor"},
}

var (
	once   sync.Once
	states []State
)

func load() {
	states = make([]State, 0, len(order))
	for _, o := range order {
		raw, err := files.ReadFile("payloads/" + o.name + ".json")
		if err != nil {
			continue
		}
		states = append(states, State{
			Name:      o.name,
			Reference: o.reference,
			Desc:      o.desc,
			Payload:   raw,
			Git:       readSidecar(o.name),
		})
	}
}

type sidecar struct {
	IsRepo bool   `json:"is_repo"`
	Branch string `json:"branch"`
}

func readSidecar(name string) gitinfo.Info {
	raw, err := files.ReadFile("payloads/" + name + ".git.json")
	if err != nil {
		return gitinfo.Info{}
	}
	var s sidecar
	if err := json.Unmarshal(raw, &s); err != nil {
		return gitinfo.Info{}
	}
	if !s.IsRepo {
		return gitinfo.Info{}
	}
	return gitinfo.Info{Found: true, Branch: s.Branch, GitDir: "/synthetic/.git"}
}

func All() []State {
	once.Do(load)
	out := make([]State, len(states))
	copy(out, states)
	return out
}

func References() []State {
	once.Do(load)
	var out []State
	for _, s := range states {
		if s.Reference {
			out = append(out, s)
		}
	}
	return out
}

func ByName(name string) (State, bool) {
	once.Do(load)
	for _, s := range states {
		if s.Name == name {
			return s, true
		}
	}
	return State{}, false
}

func Names() []string {
	once.Do(load)
	out := make([]string, 0, len(states))
	for _, s := range states {
		out = append(out, s.Name)
	}
	return out
}

func filenames() []string {
	entries, err := files.ReadDir("payloads")
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		base := strings.TrimSuffix(path.Base(e.Name()), ".json")
		if strings.HasSuffix(base, ".git") {
			continue
		}
		out = append(out, base)
	}
	sort.Strings(out)
	return out
}
