package refstate

import (
	"encoding/json"
	"testing"

	"github.com/xqsit94/cc-statusline/internal/payload"
)

// TestEveryPayloadIsListed checks `order` against the embedded directory in both
// directions.
//
// `order` is hand-written, because it carries a reading order and a description
// that a directory listing cannot. The cost of hand-writing it is that a file
// dropped into payloads/ is loaded by nothing and a name removed from the
// directory is silently skipped by init — two failures with no symptom. This is
// the test that gives them one.
func TestEveryPayloadIsListed(t *testing.T) {
	onDisk := map[string]bool{}
	for _, name := range filenames() {
		onDisk[name] = true
	}
	listed := map[string]bool{}
	for _, o := range order {
		listed[o.name] = true
	}

	for name := range onDisk {
		if !listed[name] {
			t.Errorf("payloads/%s.json is embedded but not in `order`, so nothing loads it", name)
		}
	}
	for name := range listed {
		if !onDisk[name] {
			t.Errorf("`order` names %q, which is not in payloads/", name)
		}
	}
	if len(All()) != len(order) {
		t.Errorf("All() returned %d states, `order` has %d — init skipped one",
			len(All()), len(order))
	}
}

// TestSidecarsParse is the assertion readSidecar cannot make for itself. It
// swallows a malformed sidecar and returns a zero Info, because panicking in a
// package init would blank the status line (§3.3). The swallowing has to be
// safe, which means the malformation has to fail here instead.
func TestSidecarsParse(t *testing.T) {
	for _, name := range filenames() {
		raw, err := files.ReadFile("payloads/" + name + ".git.json")
		if err != nil {
			continue // a fixture with no git state is legitimate
		}
		var s sidecar
		if err := json.Unmarshal(raw, &s); err != nil {
			t.Errorf("payloads/%s.git.json: %v", name, err)
		}
		if s.IsRepo && s.Branch == "" {
			t.Errorf("payloads/%s.git.json claims a repository with no branch", name)
		}
	}
}

// TestPayloadsDecode proves each fixture is a payload the decoder accepts.
//
// payload.Parse tolerates anything, so this is not testing the decoder — it is
// testing that a fixture is not quietly empty. A file that lost its model field
// in an edit would still render, still produce a golden, and still pass every
// width assertion, while the four §5.1 criteria it exists to state went untested.
func TestPayloadsDecode(t *testing.T) {
	for _, s := range All() {
		p, err := payload.Parse(s.Payload)
		if err != nil {
			t.Errorf("%s: %v", s.Name, err)
			continue
		}
		if name, ok := p.ModelName(); !ok || name == "" {
			t.Errorf("%s has no model.display_name", s.Name)
		}
	}
}

// TestReferenceCount pins §5.1's count. Four states, named in the document as
// acceptance criteria; a fifth added without amending §5.1 is a spec change
// wearing a fixture's clothes.
func TestReferenceCount(t *testing.T) {
	if got := len(References()); got != 4 {
		t.Errorf("References() = %d states, PRD §5.1 states 4", got)
	}
	for _, want := range []string{"normal-42", "warning-75", "danger-92", "startup"} {
		s, ok := ByName(want)
		if !ok {
			t.Errorf("no fixture named %q", want)
			continue
		}
		if !s.Reference {
			t.Errorf("%q is not marked as a §5.1 reference state", want)
		}
	}
}
