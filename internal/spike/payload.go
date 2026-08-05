package spike

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// doc is a payload parsed loosely. M0 deliberately does not define structs: the
// point of the spike is to find out what the payload actually contains, and a
// struct can only ever show what we already assumed it contains.
type doc map[string]any

func parse(b []byte) (doc, bool) {
	var d doc
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, false
	}
	return d, true
}

// at resolves a dotted path. Returns false for a missing path and for an
// explicit JSON null, which are the same thing to every caller here.
func (d doc) at(path string) (any, bool) {
	var cur any = map[string]any(d)
	for _, part := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[part]
		if !ok || cur == nil {
			return nil, false
		}
	}
	return cur, true
}

func (d doc) num(path string) (float64, bool) {
	v, ok := d.at(path)
	if !ok {
		return 0, false
	}
	f, ok := v.(float64)
	return f, ok
}

func (d doc) str(path string) (string, bool) {
	v, ok := d.at(path)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// outputTokenKeys names the current_usage fields that do NOT count toward
// context occupancy. Measured, not assumed: including output_tokens reproduces
// used_percentage in 26 of 27 real payloads, excluding it reproduces 27 of 27.
// The one disagreement was a row sitting on a rounding boundary, which is
// exactly where a wrong numerator shows itself.
var outputTokenKeys = map[string]bool{"output_tokens": true}

// occupancy returns how many tokens count toward used_percentage: the
// input-side leaves of context_window.current_usage.
//
// context_window.total_input_tokens was measured to equal this sum in 27 of 27
// payloads, so it is not a session-cumulative counter as the name suggests —
// it is the current window's input tokens, and a single-field shortcut to the
// same number. This function keeps summing the parts anyway, so a change in
// the set of parts shows up as a disagreement rather than passing silently.
func (d doc) occupancy() (float64, []string, bool) {
	v, ok := d.at("context_window.current_usage")
	if !ok {
		return 0, nil, false
	}
	m, ok := v.(map[string]any)
	if !ok {
		return 0, nil, false
	}

	var sum float64
	var parts []string
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		f, ok := m[k].(float64)
		if !ok {
			continue
		}
		label := fmt.Sprintf("%s=%.0f", k, f)
		if outputTokenKeys[k] {
			parts = append(parts, "("+label+" excluded)")
			continue
		}
		sum += f
		parts = append(parts, label)
	}
	return sum, parts, len(parts) > 0
}

// totalInput returns context_window.total_input_tokens, the field measured to
// equal occupancy(). The report cross-checks the two: if they ever diverge,
// the payload's accounting has changed underneath us.
func (d doc) totalInput() (float64, bool) {
	return d.num("context_window.total_input_tokens")
}

// probeLine is the fallback status line: the numbers M0 is measuring, live in
// the corner of the screen. `used` is what Claude Code reports; `raw` is
// occupancy/window computed from the same payload. If the two disagree,
// used_percentage is not a percentage of the raw context window — PRD §3.1.1
// question 2, answered while you work.
func probeLine(payload []byte, spooled string, spoolErr error) string {
	var fields []string

	d, parsed := parse(payload)
	if !parsed {
		fields = append(fields, fmt.Sprintf("unparsed payload (%d bytes)", len(payload)))
	} else {
		if name, ok := d.str("model.display_name"); ok {
			fields = append(fields, "◆ "+name)
		}
		if used, ok := d.num("context_window.used_percentage"); ok {
			fields = append(fields, fmt.Sprintf("used %.1f%%", used))
		} else {
			fields = append(fields, "used —")
		}
		size, hasSize := d.num("context_window.context_window_size")
		occ, _, hasOcc := d.occupancy()
		switch {
		case hasSize && hasOcc && size > 0:
			fields = append(fields, fmt.Sprintf("raw %.1f%% (%.0f/%.0f)", occ/size*100, occ, size))
		case hasSize:
			fields = append(fields, fmt.Sprintf("win %.0f", size))
		default:
			fields = append(fields, "raw —")
		}
	}

	switch {
	case spoolErr != nil:
		fields = append(fields, "spool ERR: "+spoolErr.Error())
	case spooled != "":
		fields = append(fields, fmt.Sprintf("n=%d", spoolCount()))
	}

	return strings.Join(fields, " │ ")
}

func spoolCount() int {
	dir, err := SpoolDir()
	if err != nil {
		return 0
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	return len(entries)
}
