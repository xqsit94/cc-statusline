package refstate

import (
	"encoding/json"
	"testing"

	"github.com/xqsit94/cc-statusline/internal/payload"
)

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

func TestSidecarsParse(t *testing.T) {
	for _, name := range filenames() {
		raw, err := files.ReadFile("payloads/" + name + ".git.json")
		if err != nil {
			continue
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
