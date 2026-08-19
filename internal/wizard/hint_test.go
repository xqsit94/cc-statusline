package wizard

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/xqsit94/cc-statusline/internal/config"
)

func hint(m Model) string { return m.hintText(200) }

func narrowTo(t *testing.T, m Model, cols int) Model {
	t.Helper()
	for i := 0; m.sliderWidth() > cols; i++ {
		if i > 200 {
			t.Fatalf("the slider stopped at %d, never reaching %d", m.sliderWidth(), cols)
		}
		m = press(m, "<")
	}
	return m
}

func TestTheHintTellsADropFromAnEmptySegment(t *testing.T) {
	m, _ := resize(New(testState(t, nil)), 160)
	m = press(m, "j", "j", "j")
	if s, _ := m.at(); m.refAt(s).Name != "duration" {
		t.Fatalf("the cursor is on %q, not duration", m.refAt(s).Name)
	}

	if got := hint(m); !strings.Contains(got, "on line 1") {
		t.Errorf("at width %d the hint does not place it:\n%s", m.sliderWidth(), got)
	}

	narrow := narrowTo(t, m, 70)
	if got := hint(narrow); !strings.Contains(got, "dropped") {
		t.Errorf("at width %d, where duration is gone, the hint says:\n%s",
			narrow.sliderWidth(), got)
	}

	young := m
	for range len(m.state.Sources) - 1 {
		young = press(young, "n")
	}
	if young.source().Name != "startup" {
		t.Fatalf("cycled to %q, wanted the payload with nothing in it", young.source().Name)
	}
	got := hint(young)
	if strings.Contains(got, "dropped") {
		t.Errorf("a segment the payload never filled is reported as dropped:\n%s", got)
	}
	if !strings.Contains(got, "nothing to show") {
		t.Errorf("the hint does not say the payload is why:\n%s", got)
	}

	for _, c := range []struct {
		what string
		m    Model
	}{{"narrowed", narrow}, {"startup", young}} {
		_, plain, _ := c.m.Preview()
		if strings.Contains(strings.Join(plain, "\n"), "3m") {
			t.Errorf("%s: a duration is on the line after all: %q", c.what, plain)
		}
	}
}

func TestTheHintNamesTheRowUnderTheCursor(t *testing.T) {
	m, _ := resize(New(testState(t, nil)), 160)
	for i, want := range []string{"model", "context", "cost", "duration", "ratelimits"} {
		at := press(m, strings.Split(strings.Repeat("j", i), "")...)
		got := hint(at)
		if !strings.HasPrefix(strings.TrimSpace(got), "> "+want+" — ") {
			t.Errorf("cursor %d: the hint opens with %q, want %q\n%s",
				i, strings.SplitN(strings.TrimSpace(got), "\n", 2)[0], want, got)
		}
		h, _ := helpFor(want)
		if !strings.Contains(got, h.what) {
			t.Errorf("cursor %d (%s) is described as something else:\n%s", i, want, got)
		}
	}
}

func TestEverySegmentHasAHint(t *testing.T) {
	for _, name := range config.SegmentNames {
		h, ok := helpFor(name)
		if !ok {
			t.Errorf("%s has no declaration, so the hint for it is blank", name)
			continue
		}
		if h.what == "" || h.file == "" {
			t.Errorf("%s: %+v — both halves are shown, so both are needed", name, h)
		}
	}
}

func TestTheHintNamesEveryKeyASegmentHas(t *testing.T) {
	for _, name := range config.SegmentNames {
		d, ok := config.SegmentDefOf(name)
		if !ok {
			t.Fatalf("%s is in SegmentNames with no declaration", name)
		}
		h, _ := helpFor(name)

		for _, k := range d.Keys {
			if !strings.Contains(h.file, k.Name) {
				t.Errorf("%s: the hint does not name its %q key:\n  %s", name, k.Name, h.file)
			}
		}
		for _, c := range d.Colors() {
			if !strings.Contains(h.file, c) {
				t.Errorf("%s: the hint does not name [colors] %s:\n  %s", name, c, h.file)
			}
		}
	}
}

func TestTheMarkerIsDescribedAsAGapAndNotASegment(t *testing.T) {
	m, _ := resize(New(testState(t, nil)), 160)
	m = press(m, "f")
	s, _ := m.at()
	if !m.refAt(s).IsFlex() {
		t.Fatalf("f left the cursor on %q", m.refAt(s).Name)
	}

	got := hint(m)
	if !strings.Contains(got, "not a segment") {
		t.Errorf("the marker is not described as a gap:\n%s", got)
	}
	if keys := m.keysFor(s); strings.Contains(keys, "+/-") || strings.Contains(keys, "drop") {
		t.Errorf("the keys row offers a priority the marker does not have: %q", keys)
	}
	if !strings.Contains(got, "right edge") {
		t.Errorf("the hint does not say the marker is spending width:\n%s", got)
	}
}

func TestAMarkerWithNothingToPushSaysSo(t *testing.T) {
	m, _ := resize(New(testState(t, nil)), 160)
	m = press(m, "j", "j", "f")
	for range len(m.state.Sources) - 1 {
		m = press(m, "n")
	}
	if m.source().Name != "startup" {
		t.Fatalf("cycled to %q", m.source().Name)
	}

	if got := hint(m); !strings.Contains(got, "nothing to its right") {
		t.Errorf("a marker with nothing after it reports:\n%s", got)
	}
}

func TestADisabledSegmentSaysWhereSpaceWouldPutIt(t *testing.T) {
	m, _ := resize(New(testState(t, nil)), 160)
	m = press(m, strings.Split(strings.Repeat("j", 7), "")...)
	m = press(m, "space")
	s, ok := m.at()
	if !ok || s.row != parkedRow {
		t.Fatalf("after space the cursor is on %+v, not the disabled pool", s)
	}

	got := hint(m)
	if !strings.Contains(got, "not on any row") {
		t.Errorf("a disabled segment is not reported as off:\n%s", got)
	}
	if !strings.Contains(got, "end of line 2") {
		t.Errorf("the hint does not say where space puts it:\n%s", got)
	}
}

func TestTheHintCountsRowsThatWillBeWritten(t *testing.T) {
	m, _ := resize(New(testState(t, nil)), 160)
	for range 5 {
		m = press(m, "space")
		m.cursor = 0
	}
	if len(m.rows[0].Segments) != 0 {
		t.Fatalf("line 1 still holds %v", names(m))
	}

	m.cursor = 0
	s, _ := m.at()
	if s.row != 1 || m.refAt(s).Name != "branch" {
		t.Fatalf("the cursor is on %+v (%s), want line 2's branch", s, m.refAt(s).Name)
	}
	if got := m.traceRow(1); got != 0 {
		t.Errorf("line 2 is row %d of the rendered configuration, want 0", got)
	}
	if got := hint(m); !strings.Contains(got, "on line 2") {
		t.Errorf("branch is on screen but the hint says otherwise:\n%s", got)
	}
}

func TestThePickerKeepsItsOwnAccount(t *testing.T) {
	m, _ := resize(New(testState(t, nil)), 160)
	m = openPresets(t, m)
	if got := hint(m); got != "" {
		t.Errorf("the picker is open and the hint still says:\n%s", got)
	}
	if got := m.previewPane(m.hintBox(true, 100)); !strings.Contains(got, m.state.Presets[0].Desc) {
		t.Errorf("the pane stopped describing the highlighted preset:\n%s", got)
	}
}

func TestTheHintBoxFillsThePane(t *testing.T) {
	for _, term := range []int{100, 140, 200} {
		st := testState(t, nil)
		st.Columns = 80
		m := New(st)
		m.term = term
		if !m.sideBySide() {
			t.Fatalf("term %d: the panes stacked, so there is no pane to fill", term)
		}

		var opens, closes []string
		for _, l := range strings.Split(m.View(), "\n") {
			if strings.Contains(l, "╭") {
				opens = append(opens, l)
			}
			if strings.Contains(l, "╰") {
				closes = append(closes, l)
			}
		}
		if len(opens) < 2 || len(closes) < 2 {
			t.Fatalf("term %d: no hint box in\n%s", term, m.View())
		}
		top, bottom := opens[1], closes[0]
		for _, l := range []string{top, bottom} {
			if !strings.HasSuffix(l, "╮ │") && !strings.HasSuffix(l, "╯ │") {
				t.Errorf("term %d: the hint box stops short of the pane:\n%s", term, l)
			}
		}
	}
}

func TestNothingUnderTheBoxMovesAsTheCursorDoes(t *testing.T) {
	for _, term := range []int{100, 156} {
		m, _ := resizeTo(New(testState(t, nil)), term, 60)
		_, _, available := m.Preview()
		marker := "-- " + itoa(available) + " --"

		at := -1
		for i := range m.slots() {
			cur := m
			cur.cursor = i

			row := -1
			for n, l := range strings.Split(cur.View(), "\n") {
				if strings.Contains(l, marker) {
					row = n
				}
			}
			if row < 0 {
				t.Fatalf("term %d, cursor %d: no width rule in\n%s", term, i, cur.View())
			}
			if at < 0 {
				at = row
				continue
			}
			if row != at {
				s, _ := cur.at()
				t.Errorf("term %d: the rule is on row %d with the cursor on %+v and row %d "+
					"at the top of the sidebar — the box above it changed height",
					term, row, s, at)
			}
		}
	}
}

func TestAWrappedRowContinuesUnderItsValue(t *testing.T) {
	m, _ := resize(New(testState(t, nil)), 160)
	const width = 56
	got := m.hintText(width)

	labelled := func(l string) bool {
		for _, name := range hintLabels {
			if strings.HasPrefix(l, "  "+name+" ") {
				return true
			}
		}
		return false
	}

	continuations, seenLabel := 0, false
	for i, l := range strings.Split(got, "\n") {
		if w := lipgloss.Width(l); w > width {
			t.Errorf("line %d is %d cells wide, past the %d it was given:\n%s",
				i, w, width, got)
		}
		if labelled(l) {
			seenLabel = true
			continue
		}
		if !seenLabel {
			continue
		}
		continuations++
		if !strings.HasPrefix(l, strings.Repeat(" ", labelWidth)) {
			t.Errorf("a continuation starts against the frame rather than under "+
				"the value column: %q", l)
		}
	}
	if continuations == 0 {
		t.Fatalf("nothing wrapped at %d cells, so this proves nothing:\n%s", width, got)
	}
}

func TestANarrowPaneGoesWithoutTheBox(t *testing.T) {
	m, _ := resizeTo(New(testState(t, nil)), 40, 200)
	got := m.View()
	if strings.Contains(got, "  here  ") {
		t.Errorf("a 40-column terminal drew the hint anyway:\n%s", got)
	}
	if !strings.Contains(got, "q quit") || !strings.Contains(got, "segments") {
		t.Errorf("more than the hint went missing:\n%s", got)
	}
}

func TestAShortTerminalKeepsTheHelpAndDropsTheHint(t *testing.T) {
	m, _ := resizeTo(New(testState(t, nil)), 160, 40)
	tall := m.View()
	if !strings.Contains(tall, "here") {
		t.Fatalf("no hint at 40 rows, so this test cannot show one being dropped:\n%s", tall)
	}

	short, _ := resizeTo(m, 160, 20)
	got := short.View()
	if strings.Contains(got, "  here  ") {
		t.Errorf("at 20 rows the hint is still drawn:\n%s", got)
	}
	if !strings.Contains(got, "q quit") {
		t.Errorf("the help line went instead of the hint:\n%s", got)
	}
	back, _ := resizeTo(short, 160, 40)
	if !strings.Contains(back.View(), "  here  ") {
		t.Errorf("the hint did not return when the terminal grew:\n%s", back.View())
	}
}

func TestTheLineIsMeasuredInTheSourcesOwnEnvironment(t *testing.T) {
	st := testState(t, nil)
	wide := st.Sources[0]
	wide.Name = "cjk"
	wide.Env = map[string]string{"TERM": "xterm-256color", "LC_ALL": "ja_JP.UTF-8"}
	st.Sources = []Source{st.Sources[0], wide}

	widthOf := func(m Model, plain string) int {
		return m.previewContext().Style.Width(plain)
	}

	m, _ := resize(New(st), 160)
	_, plain, _ := m.Preview()
	narrowCells := widthOf(m, plain[0])

	m = press(m, "n")
	if m.source().Name != "cjk" {
		t.Fatalf("n cycled to %q", m.source().Name)
	}
	_, plain, _ = m.Preview()
	if got := widthOf(m, plain[0]); got <= narrowCells {
		t.Errorf("the same line measures %d cells under a CJK locale and %d under "+
			"the default; the wide glyphs were measured with the narrow style",
			got, narrowCells)
	}
}
