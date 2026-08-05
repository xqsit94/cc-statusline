package payload

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// PRD §3.1.2: the highest-probability failure over twelve months is a payload
// schema change, and (value, ok) accessors turn that into a segment that
// silently disappears. Nothing on the render path can notice — the whole point
// of the failure contract is that render never complains. So the noticing
// happens here, on demand, in `doctor`.
//
// The difference this makes is the difference between a bug report and a quiet
// uninstall.

// Known is every key path this build understands, from PRD §3.1 — both the
// rendered sources and the "available but not rendered" inventory.
//
// A trailing ".*" marks a subtree whose members are not enumerated: anything
// beneath it counts as known.
var Known = []string{
	"model.id",
	"model.display_name",

	"context_window.total_input_tokens",
	"context_window.total_output_tokens",
	"context_window.context_window_size",
	"context_window.used_percentage",
	"context_window.remaining_percentage",
	"context_window.current_usage.*",

	"cost.total_cost_usd",
	"cost.total_duration_ms",
	"cost.total_api_duration_ms",
	"cost.total_lines_added",
	"cost.total_lines_removed",

	"rate_limits.five_hour.used_percentage",
	"rate_limits.five_hour.resets_at",
	"rate_limits.seven_day.used_percentage",
	"rate_limits.seven_day.resets_at",

	"workspace.current_dir",
	"workspace.project_dir",
	"workspace.added_dirs",
	"workspace.git_worktree",
	"workspace.repo.*",
	"worktree.*",
	"pr.*",

	"cwd",
	"session_id",
	"session_name",
	"prompt_id",
	"transcript_path",
	"version",
	"output_style.name",
	"effort.level",
	"thinking.enabled",
	"fast_mode",
	"exceeds_200k_tokens",
	"vim.mode",
	"agent.name",
}

// KeyDiff compares this payload's key set against Known.
//
// `unknown` is the interesting half: a key Claude Code sends that this build
// has never heard of, which is either a new feature to render or the leading
// edge of a rename.
//
// `missing` is noisier and must be presented as such. Most of Known is
// conditional — `vim.mode` needs vim mode on, `pr.*` needs a pull request,
// `rate_limits` needs a subscription — so a long missing list is the normal
// state of a single payload, not a fault. It is only evidence when a key that
// should be unconditional (`model.display_name`, `cost.total_cost_usd`) shows
// up in it.
func (p *Payload) KeyDiff() (unknown, missing []string) {
	observed, err := FlattenKeys(p.raw)
	if err != nil {
		return nil, nil
	}

	for path := range observed {
		if !isKnown(path) {
			unknown = append(unknown, path)
		}
	}

	for _, want := range Known {
		if !observedHas(observed, want) {
			missing = append(missing, want)
		}
	}

	sort.Strings(unknown)
	sort.Strings(missing)
	return unknown, missing
}

// FlattenKeys walks a JSON document and returns every leaf path mapped to its
// JSON type. Arrays are recorded at the array's own path and recursed into via
// a "[]" element, so a shape change inside a list is visible too.
func FlattenKeys(b []byte) (map[string]string, error) {
	if len(b) == 0 {
		return nil, fmt.Errorf("payload: no bytes to flatten")
	}
	var doc any
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	out := map[string]string{}
	flatten("", doc, out)
	return out, nil
}

func flatten(prefix string, v any, out map[string]string) {
	switch t := v.(type) {
	case map[string]any:
		if prefix != "" && len(t) == 0 {
			out[prefix] = "object(empty)"
			return
		}
		for k, child := range t {
			path := k
			if prefix != "" {
				path = prefix + "." + k
			}
			flatten(path, child, out)
		}
	case []any:
		out[prefix] = fmt.Sprintf("array[%d]", len(t))
		if len(t) > 0 {
			flatten(prefix+"[]", t[0], out)
		}
	case string:
		out[prefix] = "string"
	case float64:
		out[prefix] = "number"
	case bool:
		out[prefix] = "bool"
	case nil:
		out[prefix] = "null"
	default:
		out[prefix] = fmt.Sprintf("%T", v)
	}
}

func isKnown(path string) bool {
	base := strings.ReplaceAll(path, "[]", "")
	for _, want := range Known {
		if strings.HasSuffix(want, ".*") {
			if strings.HasPrefix(base, strings.TrimSuffix(want, "*")) {
				return true
			}
			continue
		}
		if base == want {
			return true
		}
	}
	return false
}

func observedHas(observed map[string]string, want string) bool {
	if !strings.HasSuffix(want, ".*") {
		_, ok := observed[want]
		return ok
	}
	prefix := strings.TrimSuffix(want, "*")
	for path := range observed {
		if strings.HasPrefix(strings.ReplaceAll(path, "[]", ""), prefix) {
			return true
		}
	}
	return false
}
