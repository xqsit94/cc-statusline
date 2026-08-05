// Package refstate holds PRD §5.1's reference payloads.
//
// # Why these are a package and not testdata
//
// Three separate things consume these payloads and only one of them is a test:
//
//	§5.1's acceptance criteria   internal/line — the four byte-identical states
//	§9.2's tier-1 goldens        internal/line — the same four across the matrix
//	§9.4's visual gate           cmd/preview   — what a human looks at
//
// They were three copies. Two of them agreed by coincidence and the third was
// written by hand from the first. A gate where the human signs off on a payload
// that is not the payload the criteria assert against validates nothing, so
// there is now one copy and everything reads it from here.
//
// This is also §10.4's "bundled fixture": the M7 wizard previews against the
// captured payload in the cache and falls back to one of these when there is
// none. `go install` leaves no repository on disk, which is why the bytes are
// embedded rather than read from a path.
//
// # Why the git state is a sidecar
//
// §9.1 wants fixtures that are captured Claude Code payloads, byte for byte,
// so that a contract change shows up as a diff against something real. Git is
// not in the payload — §3.2 — so it travels beside it in a `.git.json` file
// rather than being smuggled into a field Claude Code does not send.
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

// State is one payload plus the git state it was captured beside.
type State struct {
	// Name is the file's basename: "normal-42", "startup", …
	Name string
	// Reference marks the four states PRD §5.1 states acceptance criteria for.
	// long-model is a width-stress fixture and is not one of them.
	Reference bool
	// Desc is what the state is for, printed by `preview`.
	Desc string
	// Payload is the raw JSON, exactly as it sits on disk.
	Payload []byte
	// Git is the injected discovery result. Zero means "not a repository",
	// which is the startup state.
	Git gitinfo.Info
}

// order is the canonical sequence: the four reference states in §5.1's order,
// then the stress fixtures. Glob order is alphabetical, which would put
// `danger-92` first and `startup` between `normal-42` and `warning-75` — a
// reading order nobody chose.
//
// It is also the enumeration. Nothing walks the embedded directory to build the
// list, so a payload added to payloads/ and not added here renders nowhere;
// TestEveryPayloadIsListed is what makes that a failure rather than a silence.
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
}

// states is built on first use and never mutated afterwards.
var (
	once   sync.Once
	states []State
)

// load reads the embedded payloads.
//
// It runs on demand rather than in `init`, because this package lives in the
// same binary as `render` and §8.1 gives the whole process a 20ms p99 budget.
// Ten embedded reads and five JSON parses for a command the hot path never
// calls is a cost with no beneficiary — `init` would charge every status line
// in every session for something only the visual gate and M7's wizard use.
//
// It cannot panic, and that is not caution for its own sake: an init that died
// on a malformed sidecar would blank the user's status line with no message
// anywhere, which is precisely §3.3's failure. A sidecar that will not parse
// yields a zero Info here and fails TestSidecarsParse there.
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

// sidecar is the on-disk shape of a .git.json file.
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
	// A synthetic GitDir, because the goldens must not depend on the branch the
	// developer happens to be standing on. §9.1: git is injected, never
	// discovered, or every golden is flaky by construction.
	return gitinfo.Info{Found: true, Branch: s.Branch, GitDir: "/synthetic/.git"}
}

// All returns every fixture in canonical order.
func All() []State {
	once.Do(load)
	out := make([]State, len(states))
	copy(out, states)
	return out
}

// References returns only the four states §5.1 states criteria for.
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

// ByName resolves one fixture. `preview --state danger-92`.
func ByName(name string) (State, bool) {
	once.Do(load)
	for _, s := range states {
		if s.Name == name {
			return s, true
		}
	}
	return State{}, false
}

// Names lists the fixtures in canonical order, for usage text.
func Names() []string {
	once.Do(load)
	out := make([]string, 0, len(states))
	for _, s := range states {
		out = append(out, s.Name)
	}
	return out
}

// filenames lists the embedded payloads, ignoring sidecars. Only the test uses
// it — it is the "from the other side" half of `order` being hand-written.
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
