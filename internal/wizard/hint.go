package wizard

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/xqsit94/cc-statusline/internal/config"
	"github.com/xqsit94/cc-statusline/internal/line"
)

type help struct{ what, file string }

func helpFor(name string) (help, bool) {
	d, ok := config.SegmentDefOf(name)
	if !ok {
		return help{}, false
	}

	parts := append([]string{}, d.Tunes...)

	keys := make([]string, 0, len(d.Keys))
	for _, k := range d.Keys {
		keys = append(keys, k.Name)
	}
	parts = append(parts, "[segments."+d.Name+"] "+strings.Join(keys, ", "))

	if colors := d.Colors(); len(colors) > 0 {
		parts = append(parts, "[colors] "+strings.Join(colors, ", "))
	}

	return help{what: d.Doc, file: strings.Join(parts, " · ")}, true
}

const (
	flexWhat = "not a segment but a gap: the width the row did not use. " +
		"Everything after it moves to the right edge."
	flexFile = `{name="flex"} in the row's segments list. It takes no drop.`

	presetsWhat = "the bundled layouts. Taking one replaces your rows and the " +
		"icons, powerline and colour keys — nothing else."
	presetsFile = "cc-statusline init --preset <name> writes one out as a file"
)

func (m Model) hintText(width int) string {
	if m.mode == modePresets {
		return ""
	}
	s, ok := m.at()
	if !ok {
		return ""
	}

	var title, what, file string
	switch {
	case s.row == presetsRow:
		title, what, file = "presets", presetsWhat, presetsFile
	case m.refAt(s).IsFlex():
		title, what, file = config.FlexName, flexWhat, flexFile
	default:
		h, ok := helpFor(m.refAt(s).Name)
		if !ok {
			return ""
		}
		title, what, file = m.refAt(s).Name, h.what, h.file
	}

	rows := []string{
		wrapped(headingStyle.Render("> "+title)+dimStyle.Render(" — "+what), 0, width),
	}
	if where := m.whereItIs(s); where != "" {
		rows = append(rows, labelled("here", where, width))
	}
	if f := m.formatOf(s); f != "" {
		rows = append(rows, labelled("format", dimStyle.Render(f), width))
	}
	rows = append(rows, labelled("keys", dimStyle.Render(m.keysFor(s)), width))
	if file != "" {
		rows = append(rows, labelled("file", dimStyle.Render(file), width))
	}
	return strings.Join(rows, "\n")
}

var hintLabels = []string{"here", "format", "keys", "file"}

var labelWidth = func() int {
	w := 0
	for _, l := range hintLabels {
		w = max(w, lipgloss.Width(l))
	}
	return w + 4
}()

func labelled(label, text string, width int) string {
	gutter := strings.Repeat(" ", max(1, labelWidth-2-lipgloss.Width(label)))
	return dimStyle.Render("  "+label+gutter) + wrapped(text, labelWidth, width)
}

func wrapped(text string, indent, width int) string {
	body := lipgloss.NewStyle().Width(max(1, width-indent)).Render(text)
	lines := strings.Split(body, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
		if i > 0 {
			lines[i] = strings.Repeat(" ", indent) + lines[i]
		}
	}
	return strings.Join(lines, "\n")
}

const minHintBox = 44

func (m Model) hintBox(show bool, paneWidth int) string {
	if !show {
		return ""
	}
	inner := paneWidth - paneStyle.GetHorizontalPadding() - border
	if inner < minHintBox {
		return ""
	}
	width := inner - hintStyle.GetHorizontalPadding()
	body := m.hintText(width)
	if body == "" {
		return ""
	}
	return hintStyle.Width(inner).Height(m.hintRows(width)).Render(body)
}

func (m Model) hintRows(width int) int {
	rows := 1
	for i := range m.slots() {
		m.cursor = i
		rows = max(rows, lipgloss.Height(m.hintText(width)))
	}
	return rows
}

func (m Model) refAt(s slot) config.SegmentRef {
	if s.row == parkedRow {
		return m.parked[s.idx]
	}
	return m.rows[s.row].Segments[s.idx]
}

func (m Model) whereItIs(s slot) string {
	switch {
	case s.row == presetsRow:
		return ""
	case s.row == parkedRow:
		return "not on any row"
	}

	p, ok := m.placementAt(s)
	if !ok {
		return ""
	}
	row := fmt.Sprintf("line %d", s.row+1)

	if m.refAt(s).IsFlex() {
		if p == line.Shown {
			return "on " + row + ", holding everything after it against the right edge"
		}
		return "not drawn — nothing to its right on " + row + " to push"
	}

	switch p {
	case line.Shown:
		return "on " + row
	case line.Truncated:
		return "on " + row + ", shortened by the fitter to make the row fit"
	case line.Dropped:
		return fmt.Sprintf("dropped — %s is over budget at width %d, and a lower drop lasts longer",
			row, m.sliderWidth())
	default:
		return fmt.Sprintf("nothing to show for the %s payload — n cycles to another",
			m.source().Name)
	}
}

func (m Model) formatOf(s slot) string {
	if s.row == presetsRow || m.refAt(s).IsFlex() {
		return ""
	}
	name := m.refAt(s).Name
	ring := m.ringFor(name)
	cur := config.VariantOf(name, m.formats)
	if len(ring) < 2 {
		return leadFormat(cur)
	}
	return fmt.Sprintf("%d of %d · %s",
		max(0, config.IndexOfVariant(ring, name, m.formats))+1, len(ring), leadFormat(cur))
}

func (m Model) placementAt(s slot) (line.Placement, bool) {
	if s.row < 0 {
		return line.Absent, false
	}
	trace := line.Trace(m.previewContext())
	r := m.traceRow(s.row)
	if r >= len(trace) || s.idx >= len(trace[r]) {
		return line.Absent, false
	}
	return trace[r][s.idx], true
}

func (m Model) traceRow(row int) int {
	n := 0
	for _, r := range m.rows[:row] {
		if len(r.Segments) > 0 {
			n++
		}
	}
	return n
}

func (m Model) keysFor(s slot) string {
	switch {
	case s.row == presetsRow:
		return "enter opens the picker · j/k previews each one before you take it"
	case s.row == parkedRow:
		return fmt.Sprintf("space puts it at the end of line %d · tab changes its format · J/K then moves it",
			max(1, len(m.rows)))
	case m.refAt(s).IsFlex():
		return "space removes it · J/K moves it along the row"
	}
	return "space disables it · tab changes its format · +/- changes its drop · " +
		"J/K moves it · f adds a gap after it"
}
