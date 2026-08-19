package spike

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

type doc map[string]any

func parse(b []byte) (doc, bool) {
	var d doc
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, false
	}
	return d, true
}

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

var outputTokenKeys = map[string]bool{"output_tokens": true}

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

func (d doc) totalInput() (float64, bool) {
	return d.num("context_window.total_input_tokens")
}

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
