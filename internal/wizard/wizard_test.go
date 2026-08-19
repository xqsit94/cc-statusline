package wizard

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/xqsit94/cc-statusline/internal/config"
	"github.com/xqsit94/cc-statusline/internal/line"
	"github.com/xqsit94/cc-statusline/internal/payload"
	"github.com/xqsit94/cc-statusline/internal/refstate"
	"github.com/xqsit94/cc-statusline/internal/style"
)

type saveRecorder struct {
	calls []Result
	err   error
}

func (s *saveRecorder) fn(r Result) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	s.calls = append(s.calls, r)
	return "saved /tmp/config.toml", nil
}

func testState(t *testing.T, save func(Result) (string, error)) State {
	t.Helper()
	var sources []Source
	for _, st := range refstate.References() {
		p, err := payload.Parse(st.Payload)
		if err != nil {
			t.Fatalf("%s: %v", st.Name, err)
		}
		sources = append(sources, Source{Name: st.Name, Desc: st.Desc, Payload: p, Git: st.Git})
	}
	return State{
		Config:  config.Defaults(),
		Env:     map[string]string{"TERM": "xterm-256color", "COLORTERM": "truecolor"},
		Sources: sources,
		Presets: testPresets(),
		Columns: 120,
		Save:    save,
		Apply: testApply(func() (string, error) {
			return testSettings + " — installed", nil
		}),
	}
}

const testSettings = "/home/u/.claude/settings.json"

func testApply(do func() (string, error)) Apply {
	return Apply{Target: testSettings, Do: do}
}

type writeLog struct {
	steps    []string
	saves    []Result
	saveErr  error
	applyErr error
}

func (w *writeLog) save(r Result) (string, error) {
	w.steps = append(w.steps, "save")
	if w.saveErr != nil {
		return "", w.saveErr
	}
	w.saves = append(w.saves, r)
	return "saved /tmp/config.toml", nil
}

func (w *writeLog) apply() (string, error) {
	w.steps = append(w.steps, "apply")
	if w.applyErr != nil {
		return "", w.applyErr
	}
	return testSettings + " — installed", nil
}

func logged(t *testing.T, w *writeLog) State {
	t.Helper()
	st := testState(t, w.save)
	st.Apply = testApply(w.apply)
	return st
}

func testPresets() []Preset {
	two := config.Defaults().Lines
	one := []config.Line{{Segments: []config.SegmentRef{
		{Name: "context", Drop: config.NeverDrop},
		{Name: "branch", Drop: config.NeverDrop},
	}}}
	return []Preset{
		{Name: "default", Desc: "the commented reference configuration.",
			Result: Result{Lines: two, Icons: "unicode", Powerline: "auto", Colour: "auto"}},
		{Name: "minimal", Desc: "one line, nothing that moves on its own.",
			Result: Result{Lines: one, Icons: "ascii", Powerline: "false", Colour: "256"}},
	}
}

func press(m Model, keys ...string) Model {
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "space":
			msg = tea.KeyMsg{Type: tea.KeySpace}
		case "up", "down", "left", "right", "enter", "esc", "tab", "shift+tab", "ctrl+s":
			msg = tea.KeyMsg{Type: keyTypes[k]}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

var keyTypes = map[string]tea.KeyType{
	"up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight,
	"enter": tea.KeyEnter, "esc": tea.KeyEsc,
	"tab": tea.KeyTab, "shift+tab": tea.KeyShiftTab,
	"ctrl+s": tea.KeyCtrlS,
}

func seek(t *testing.T, m Model, name string) Model {
	t.Helper()
	for i, s := range m.slots() {
		if s.row == presetsRow {
			continue
		}
		if m.refAt(s).Name == name {
			m.cursor = i
			return m
		}
	}
	t.Fatalf("no sidebar row for %q", name)
	return m
}

func names(m Model) []string {
	var out []string
	for i, r := range m.rows {
		if i > 0 {
			out = append(out, "|")
		}
		for _, s := range r.Segments {
			out = append(out, s.Name)
		}
	}
	return out
}

func TestPreviewIsWhatRenderProduces(t *testing.T) {
	for _, icons := range iconCycle {
		for i := range refstate.References() {
			m := New(testState(t, nil))
			m.icons = icons
			m.sourceIdx = i

			got, _, _ := m.Preview()

			cfg := m.previewConfig()
			caps := style.Detect(style.Overlay(m.state.Env, style.Overrides{
				Icons: icons, Columns: m.width,
			}), cfg)
			src := m.source()
			want := line.Render(line.Context{
				Payload: src.Payload, Git: src.Git, Config: cfg,
				Style: style.NewStyle(caps, cfg),
			})

			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s/%s: the pane and the render path disagree\n got: %q\nwant: %q",
					icons, src.Name, got, want)
			}
		}
	}
}

func TestReorderCrossesRowBoundaries(t *testing.T) {
	m := New(testState(t, nil))
	before := names(m)
	firstRowLen := len(m.rows[0].Segments)

	m = press(m, strings.Split(strings.Repeat("j", firstRowLen-1), "")...)
	m = press(m, "J")

	if len(m.rows[0].Segments) != firstRowLen-1 {
		t.Errorf("row 1 has %d segments, want %d", len(m.rows[0].Segments), firstRowLen-1)
	}
	if got := m.rows[1].Segments[0].Name; got != before[firstRowLen-1] {
		t.Errorf("row 2 starts with %q, want the segment that moved (%q)", got, before[firstRowLen-1])
	}
	if !m.dirty {
		t.Error("a reorder did not mark the model dirty")
	}

	m = press(m, "K")
	if got := names(m); !reflect.DeepEqual(got, before) {
		t.Errorf("K did not undo J\n got: %v\nwant: %v", got, before)
	}
}

func TestReorderCarriesTheCursor(t *testing.T) {
	selected := func(m Model) string {
		s, ok := m.at()
		if !ok {
			t.Fatal("no segment is selected")
		}
		if s.row == parkedRow {
			return m.parked[s.idx].Name
		}
		return m.rows[s.row].Segments[s.idx].Name
	}

	m := New(testState(t, nil))
	first := selected(m)

	m = press(m, "J")
	if got := selected(m); got != first {
		t.Errorf("after J the cursor is on %q, want the segment that moved (%q)", got, first)
	}
	if got := m.rows[0].Segments[1].Name; got != first {
		t.Fatalf("J did not move %q to index 1; row is %v", first, names(m))
	}
	m = press(m, "J")
	if got := selected(m); got != first {
		t.Errorf("after a second J the cursor is on %q, want %q", got, first)
	}
	if got := m.rows[0].Segments[2].Name; got != first {
		t.Errorf("the second J moved %q instead of %q", names(m)[1], first)
	}

	m = New(testState(t, nil))
	m = press(m, strings.Split(strings.Repeat("j", len(m.rows[0].Segments)), "")...)
	moved := selected(m)
	m = press(m, "K")
	if got := selected(m); got != moved {
		t.Errorf("after K across the boundary the cursor is on %q, want %q", got, moved)
	}
	last := m.rows[0].Segments[len(m.rows[0].Segments)-1].Name
	if last != moved {
		t.Errorf("K put %q at the end of row 1, want %q", last, moved)
	}
}

func TestReorderAtTheEndsDoesNothing(t *testing.T) {
	m := New(testState(t, nil))
	before := names(m)

	m = press(m, "K", "K", "K")
	if got := names(m); !reflect.DeepEqual(got, before) {
		t.Errorf("K at the top changed the layout\n got: %v\nwant: %v", got, before)
	}

	last := len(m.rows) - 1
	m.seek(slot{last, len(m.rows[last].Segments) - 1})
	m = press(m, "J", "J", "J")
	if got := names(m); !reflect.DeepEqual(got, before) {
		t.Errorf("J at the bottom changed the layout\n got: %v\nwant: %v", got, before)
	}
}

func TestDisableAndReEnableKeepsEverySegment(t *testing.T) {
	m := New(testState(t, nil))

	count := func(m Model) map[string]int {
		c := map[string]int{}
		for _, r := range m.rows {
			for _, s := range r.Segments {
				c[s.Name]++
			}
		}
		for _, s := range m.parked {
			c[s.Name]++
		}
		return c
	}
	before := count(m)

	m = press(m, "space", "space", "space")
	if len(m.parked) < 3 {
		t.Fatalf("three toggles parked %d segments", len(m.parked))
	}
	for len(m.parked) > 0 {
		m.seek(slot{parkedRow, len(m.parked) - 1})
		m = press(m, "space")
	}

	if got := count(m); !reflect.DeepEqual(got, before) {
		t.Errorf("a segment was lost or duplicated\n got: %v\nwant: %v", got, before)
	}
	for name, n := range count(m) {
		if n != 1 {
			t.Errorf("%s appears %d times", name, n)
		}
	}
}

func TestDropClampsToTheSchema(t *testing.T) {
	m := New(testState(t, nil))

	m = press(m, strings.Split(strings.Repeat("+", 200), "")...)
	if got := m.rows[0].Segments[0].Drop; got != config.NeverDrop {
		t.Errorf("drop clamped at %d, want %d", got, config.NeverDrop)
	}
	m = press(m, strings.Split(strings.Repeat("-", 200), "")...)
	if got := m.rows[0].Segments[0].Drop; got != 0 {
		t.Errorf("drop floored at %d, want 0", got)
	}
}

func TestResetReturnsToTheLoadedState(t *testing.T) {
	m := New(testState(t, nil))
	want := names(m)
	wantIcons := m.icons

	m = press(m, "j", "J", "space", "+", "+", "i", "i", "p", "c")
	if !m.dirty {
		t.Fatal("nine edits did not mark the model dirty")
	}
	m = press(m, "r")

	if got := names(m); !reflect.DeepEqual(got, want) {
		t.Errorf("reset left the layout as\n got: %v\nwant: %v", got, want)
	}
	if m.icons != wantIcons {
		t.Errorf("reset left icons as %q, want %q", m.icons, wantIcons)
	}
	if m.dirty {
		t.Error("still dirty after a reset")
	}
	if len(m.parked) != len(New(testState(t, nil)).parked) {
		t.Errorf("the disabled pool did not reset")
	}
}

func TestQuitNeverSaves(t *testing.T) {
	rec := &saveRecorder{}
	m := press(New(testState(t, rec.fn)), "j", "J", "+", "q")
	if !m.Quit() {
		t.Error("q did not quit")
	}
	if len(rec.calls) != 0 {
		t.Errorf("q wrote %d times", len(rec.calls))
	}
	if !m.Dirty() {
		t.Error("quitting with unsaved edits did not report them; the caller cannot warn")
	}
}

func TestSaveWritesOnceAndClearsDirty(t *testing.T) {
	rec := &saveRecorder{}
	m := press(New(testState(t, rec.fn)), "j", "J", "s")

	if len(rec.calls) != 1 {
		t.Fatalf("s wrote %d times, want 1", len(rec.calls))
	}
	if m.Dirty() {
		t.Error("still dirty after a save")
	}
	if !strings.Contains(m.status, "saved") {
		t.Errorf("status after a save is %q", m.status)
	}

	after := names(m)
	m = press(m, "r")
	if got := names(m); !reflect.DeepEqual(got, after) {
		t.Errorf("reset after a save reverted the write\n got: %v\nwant: %v", got, after)
	}
}

func TestSaveSurfacesTheError(t *testing.T) {
	rec := &saveRecorder{err: errors.New("cannot rewrite the [[line]] blocks: it holds a comment")}
	m := press(New(testState(t, rec.fn)), "j", "J", "s")

	if !strings.Contains(m.err, "[[line]]") {
		t.Errorf("the error is not on screen: %q", m.err)
	}
	if !m.Dirty() {
		t.Error("a failed save cleared the dirty flag; the work looks written and is not")
	}
	if !strings.Contains(m.View(), "[[line]]") {
		t.Error("View() does not show the error")
	}
}

func TestSaveIsUnavailableRatherThanSilent(t *testing.T) {
	m := press(New(testState(t, nil)), "j", "J", "s")
	if m.err == "" {
		t.Error("s with no save function reported nothing")
	}
	if !m.Dirty() {
		t.Error("the edits were marked saved")
	}
}

func TestCtrlSAsksBeforeItWritesAnything(t *testing.T) {
	log := &writeLog{}
	m := press(New(logged(t, log)), "j", "J", "ctrl+s")

	if m.mode != modeApply {
		t.Fatalf("ctrl+s did not open the confirmation; mode is %v", m.mode)
	}
	if len(log.steps) != 0 {
		t.Errorf("ctrl+s wrote before asking: %v", log.steps)
	}
	if !m.Dirty() {
		t.Error("a prompt marked the edits saved")
	}

	view := m.View()
	if !strings.Contains(view, testSettings) {
		t.Errorf("the confirmation does not name the file it will write:\n%s", view)
	}
	for _, want := range []string{"save and apply", "cancel"} {
		if !strings.Contains(view, want) {
			t.Errorf("the confirmation does not offer %q:\n%s", want, view)
		}
	}
}

func TestConfirmingSavesThenApplies(t *testing.T) {
	log := &writeLog{}
	m := press(New(logged(t, log)), "j", "J", "ctrl+s", "y")

	if want := []string{"save", "apply"}; !reflect.DeepEqual(log.steps, want) {
		t.Fatalf("ctrl+s ran %v, want %v", log.steps, want)
	}
	if m.mode != modeSegments {
		t.Error("the confirmation stayed open after it was answered")
	}
	if m.Dirty() {
		t.Error("still dirty after a save and apply")
	}

	view := m.View()
	for _, want := range []string{"saved", testSettings} {
		if !strings.Contains(view, want) {
			t.Errorf("the footer does not mention %q:\n%s", want, view)
		}
	}
}

func TestCancellingTheConfirmationWritesNothing(t *testing.T) {
	for _, key := range []string{"n", "esc", "q", "j"} {
		t.Run(key, func(t *testing.T) {
			log := &writeLog{}
			before := press(New(logged(t, log)), "j", "J", "ctrl+s")
			after := press(before, key)

			if len(log.steps) != 0 {
				t.Errorf("%q wrote %v", key, log.steps)
			}
			if after.mode != modeSegments {
				t.Errorf("%q left the confirmation open", key)
			}
			if !after.Dirty() {
				t.Errorf("%q marked the edits saved", key)
			}
			if after.Quit() {
				t.Errorf("%q quit the wizard from inside a prompt", key)
			}
			if after.sourceIdx != before.sourceIdx {
				t.Errorf("%q answered the prompt and moved the preview behind it", key)
			}
		})
	}
}

func TestApplyIsNotAttemptedWhenTheSaveFails(t *testing.T) {
	log := &writeLog{saveErr: errors.New("cannot rewrite the [[line]] blocks: it holds a comment")}
	m := press(New(logged(t, log)), "j", "J", "ctrl+s", "y")

	if want := []string{"save"}; !reflect.DeepEqual(log.steps, want) {
		t.Fatalf("ctrl+s ran %v after a failed save, want %v", log.steps, want)
	}
	if !strings.Contains(m.err, "[[line]]") {
		t.Errorf("the save's error is not on screen: %q", m.err)
	}
	if !m.Dirty() {
		t.Error("a failed save cleared the dirty flag; the work looks written and is not")
	}
}

func TestAFailedInstallKeepsTheSuccessfulSaveOnScreen(t *testing.T) {
	log := &writeLog{applyErr: errors.New("settings.json is not plain JSON")}
	m := press(New(logged(t, log)), "j", "J", "ctrl+s", "y")

	if len(log.saves) != 1 {
		t.Fatalf("the save ran %d times", len(log.saves))
	}
	if m.Dirty() {
		t.Error("the save landed but the model still reports unsaved work")
	}
	view := m.View()
	if !strings.Contains(view, "saved") {
		t.Errorf("the install's failure hid the save that worked:\n%s", view)
	}
	if !strings.Contains(view, "plain JSON") {
		t.Errorf("the install's failure is not on screen:\n%s", view)
	}
}

func TestCtrlSIsUnavailableRatherThanSilent(t *testing.T) {
	log := &writeLog{}
	st := logged(t, log)
	st.Apply = Apply{}
	m := press(New(st), "j", "J", "ctrl+s")

	if m.mode == modeApply {
		t.Error("it opened a confirmation for an install that cannot happen")
	}
	if m.err == "" {
		t.Error("ctrl+s with nowhere to install reported nothing")
	}
	if len(log.steps) != 0 {
		t.Errorf("it wrote anyway: %v", log.steps)
	}
	if !m.Dirty() {
		t.Error("the edits were marked saved")
	}
}

func TestTheHelpFitsEightyColumns(t *testing.T) {
	m := New(testState(t, nil))
	limit := 80 - helpStyle.GetHorizontalFrameSize()
	for _, md := range []mode{modeSegments, modePresets, modeApply} {
		m.mode = md
		for _, l := range strings.Split(m.help(), "\n") {
			if w := lipgloss.Width(l); w > limit {
				t.Errorf("mode %d: a help line is %d cells, over the %d that fit: %q",
					md, w, limit, l)
			}
		}
	}
}

func TestTheHelpIsOneHeightInEveryMode(t *testing.T) {
	m := New(testState(t, nil))
	m.mode = modeSegments
	want := lipgloss.Height(m.help())
	for _, md := range []mode{modePresets, modeApply} {
		m.mode = md
		if got := lipgloss.Height(m.help()); got != want {
			t.Errorf("mode %d has a %d-line help block, want %d", md, got, want)
		}
	}
}

func TestResultDropsEmptiedRows(t *testing.T) {
	m := New(testState(t, nil))
	rows := len(m.rows)

	for len(m.rows[0].Segments) > 0 {
		m.cursor = 0
		m = press(m, "space")
	}

	if got := len(m.Result().Lines); got != rows-1 {
		t.Errorf("Result has %d rows, want %d", got, rows-1)
	}
	if !strings.Contains(m.View(), "will not be written") {
		t.Error("the view does not say the emptied row is going away")
	}
}

func TestCapabilityTogglesGoThroughDetect(t *testing.T) {
	m := New(testState(t, nil))
	m.icons = "ascii"
	m.powerline = "true"

	cfg := m.previewConfig()
	caps := style.Detect(style.Overlay(m.state.Env, style.Overrides{
		Icons: m.icons, Columns: m.width,
	}), cfg)
	if caps.Powerline {
		t.Error("Powerline resolved on under ascii; §6.1's precedence was bypassed")
	}

	m.icons = "unicode"
	unicodeOut, _, _ := m.Preview()
	m.icons = "ascii"
	asciiOut, _, _ := m.Preview()
	if reflect.DeepEqual(unicodeOut, asciiOut) {
		t.Error("the icon toggle changed nothing in the preview")
	}
}

func TestNoColorIsReportedRatherThanHidden(t *testing.T) {
	st := testState(t, nil)
	st.Env = map[string]string{"TERM": "xterm-256color", "NO_COLOR": "1"}
	m := New(st)
	m.colour = "truecolor"

	if !m.NoColorIsSet() {
		t.Fatal("NO_COLOR in the environment was not noticed")
	}
	if !strings.Contains(m.View(), "NO_COLOR") {
		t.Error("the view does not warn that NO_COLOR overrides the toggle")
	}
	rendered, _, _ := m.Preview()
	for _, l := range rendered {
		if strings.Contains(l, "\x1b[") {
			t.Errorf("the preview emitted colour under NO_COLOR: %q", l)
		}
	}
}

func TestWidthSliderIsNotWrittenToTheConfig(t *testing.T) {
	rec := &saveRecorder{}
	m := press(New(testState(t, rec.fn)), "<", "<", "<", "s")

	if m.width >= 120 {
		t.Fatalf("the slider did not move: %d", m.width)
	}
	if len(rec.calls) != 1 {
		t.Fatalf("saved %d times", len(rec.calls))
	}
	if reflect.TypeOf(rec.calls[0]).NumField() != 5 {
		t.Errorf("Result gained a field; check that the slider is still a lens")
	}
	if m.previewConfig().General.MaxWidth != 0 {
		t.Error("previewConfig pinned max_width")
	}
}

func TestNarrowTerminalStacksThePanes(t *testing.T) {
	st := testState(t, nil)
	st.Columns = 60
	if v := New(st).View(); strings.Contains(v, "segments") && !strings.Contains(v, "preview") {
		t.Error("the narrow layout dropped a pane rather than stacking it")
	}

	wide := New(testState(t, nil)).View()
	for _, want := range []string{"segments", "preview", "drop", "payload"} {
		if !strings.Contains(wide, want) {
			t.Errorf("the wide layout is missing %q", want)
		}
	}
}

func TestChromeFillsTheTerminal(t *testing.T) {
	for _, term := range []int{100, 140, 200} {
		st := testState(t, nil)
		st.Columns = 80
		m := New(st)
		m.term = term

		widest := 0
		for _, line := range strings.Split(m.View(), "\n") {
			w := lipgloss.Width(line)
			if w > term {
				t.Errorf("term %d: a line is %d cells wide, so it wraps", term, w)
			}
			widest = max(widest, w)
		}
		if widest != term {
			t.Errorf("term %d: the widest line is %d, so the chrome leaves %d columns unused",
				term, widest, term-widest)
		}
	}
}

func TestThePreviewFillsThePane(t *testing.T) {
	for _, term := range []int{100, 140, 200} {
		st := testState(t, nil)
		st.Columns = 80
		m := New(st)
		m.term = term

		if !m.sideBySide() {
			t.Fatalf("term %d: the panes stacked, so there is no pane to fill", term)
		}
		_, _, available := m.Preview()

		var rule string
		for _, l := range strings.Split(m.View(), "\n") {
			if strings.Contains(l, "-- "+itoa(available)+" --") {
				rule = l
			}
		}
		if rule == "" {
			t.Fatalf("term %d: no width rule for %d cells in\n%s", term, available, m.View())
		}
		if !strings.HasSuffix(rule, "| \u2502") {
			t.Errorf("term %d: the rule stops short of the frame:\n%s", term, rule)
		}
	}
}

func TestThePreviewDoesNotCountCellsUnderEachRow(t *testing.T) {
	m, _ := resize(New(testState(t, nil)), 160)
	rendered, _, available := m.Preview()
	if len(rendered) == 0 {
		t.Fatal("nothing previewed, so this proves nothing")
	}

	view := m.View()
	if strings.Contains(view, "cells") {
		t.Errorf("the pane counts cells again:\n%s", view)
	}
	if !strings.Contains(view, "-- "+itoa(available)+" --") {
		t.Errorf("the width rule went with the counts:\n%s", view)
	}
}

func TestTheSliderFollowsTheTerminalUntilItIsMoved(t *testing.T) {
	st := testState(t, nil)
	st.Columns = 80
	m := New(st)

	m, _ = resize(m, 200)
	wide := budget(m)
	m, _ = resize(m, 120)
	if narrower := budget(m); narrower >= wide {
		t.Errorf("previewing %d cells at 120 columns and %d at 200; it did not follow",
			narrower, wide)
	}

	m = press(m, "<")
	pinned := budget(m)
	m, _ = resize(m, 200)
	if got := budget(m); got != pinned {
		t.Errorf("a resize moved the pinned slider from %d to %d", pinned, got)
	}
}

func TestANarrowTerminalPreviewsItself(t *testing.T) {
	st := testState(t, nil)
	st.Columns = 80
	m := New(st)
	m, _ = resize(m, 70)

	if got, want := m.sliderWidth(), 70; got != want {
		t.Errorf("previewing %d columns in a %d-column terminal", got, want)
	}
}

func resize(m Model, w int) (Model, tea.Cmd) { return resizeTo(m, w, 40) }

func resizeTo(m Model, w, h int) (Model, tea.Cmd) {
	next, cmd := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return next.(Model), cmd
}

func budget(m Model) int {
	_, _, available := m.Preview()
	return available
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }

func openPresets(t *testing.T, m Model) Model {
	t.Helper()
	m = press(m, strings.Split(strings.Repeat("j", len(m.slots())), "")...)
	s, ok := m.at()
	if !ok || s.row != presetsRow {
		t.Fatalf("the bottom of the sidebar is %+v, not the presets button", s)
	}
	if m = press(m, "enter"); m.mode != modePresets {
		t.Fatal("enter on the button did not open the picker")
	}
	return m
}

func TestPresetsButtonOpensThePicker(t *testing.T) {
	m := New(testState(t, nil))
	if !strings.Contains(m.View(), "Presets") {
		t.Error("the sidebar does not offer the button")
	}

	m = openPresets(t, m)
	view := m.View()
	for _, want := range []string{"presets", "default", "minimal", "1 line", "2 lines"} {
		if !strings.Contains(view, want) {
			t.Errorf("the picker does not show %q", want)
		}
	}
	if !strings.Contains(view, "payload") {
		t.Error("the picker hid the preview pane")
	}

	m = press(New(testState(t, nil)), strings.Split(strings.Repeat("j", 99), "")...)
	if m = press(m, "l"); m.mode != modePresets {
		t.Error("l on the button did not open the picker")
	}
}

func TestPresetPickerPreviewsWithoutApplying(t *testing.T) {
	m := New(testState(t, nil))
	before, wasDirty := names(m), m.dirty
	twoRows, _, _ := m.Preview()

	m = press(openPresets(t, m), "j")
	if p, _ := m.highlighted(); p.Name != "minimal" {
		t.Fatalf("j moved the picker to %q, want minimal", p.Name)
	}

	oneRow, _, _ := m.Preview()
	if len(twoRows) == len(oneRow) {
		t.Fatalf("the preview did not follow the picker; both render %d rows", len(oneRow))
	}
	if len(oneRow) != 1 {
		t.Errorf("previewing minimal rendered %d rows, want 1", len(oneRow))
	}
	if !strings.Contains(m.View(), "icons ascii") {
		t.Error("the preview claims the edited icon set rather than the preset's")
	}

	if got := names(m); !reflect.DeepEqual(got, before) {
		t.Errorf("browsing changed the layout\n got: %v\nwant: %v", got, before)
	}
	if m.dirty != wasDirty {
		t.Error("browsing marked the model dirty")
	}

	m = press(m, "esc")
	if m.mode != modeSegments {
		t.Fatal("esc did not close the picker")
	}
	if got, _, _ := m.Preview(); !reflect.DeepEqual(got, twoRows) {
		t.Error("the preview did not return to the edited state")
	}
}

func TestApplyingAPresetIsAnEditLikeAnyOther(t *testing.T) {
	rec := &saveRecorder{}
	m := New(testState(t, rec.fn))
	loaded := names(m)

	m = press(openPresets(t, m), "j", "enter")

	if m.mode != modeSegments {
		t.Error("applying left the picker open")
	}
	if !m.dirty {
		t.Error("applying a preset did not mark the model dirty")
	}
	if len(rec.calls) != 0 {
		t.Errorf("applying wrote to disk %d times", len(rec.calls))
	}
	if len(m.rows) != 1 {
		t.Fatalf("applied minimal and got %d rows, want 1", len(m.rows))
	}
	if m.icons != "ascii" {
		t.Errorf("the preset's icon set did not come across: %q", m.icons)
	}
	if len(m.parked) != len(config.SegmentNames)-2 {
		t.Errorf("the disabled pool holds %d, want %d", len(m.parked), len(config.SegmentNames)-2)
	}

	m = press(m, "s")
	if len(rec.calls) != 1 {
		t.Fatalf("s wrote %d times", len(rec.calls))
	}
	if got := len(rec.calls[0].Lines); got != 1 {
		t.Errorf("the save carried %d rows, want the preset's 1", got)
	}

	back := press(openPresets(t, New(testState(t, nil))), "j", "enter", "r")
	if got := names(back); !reflect.DeepEqual(got, loaded) {
		t.Errorf("r did not undo the preset\n got: %v\nwant: %v", got, loaded)
	}
}

func TestWhatThePickerShowsIsWhatApplyingDoes(t *testing.T) {
	base := openPresets(t, New(testState(t, nil)))
	for i, p := range base.state.Presets {
		m := base
		m.presetIdx = i
		shown, _, shownAvailable := m.Preview()

		m.applyPreset()
		got, _, gotAvailable := m.Preview()

		if !reflect.DeepEqual(got, shown) {
			t.Errorf("%s: applying rendered something other than the preview\n got: %q\nwant: %q",
				p.Name, got, shown)
		}
		if gotAvailable != shownAvailable {
			t.Errorf("%s: the width budget moved on apply: %d, want %d",
				p.Name, gotAvailable, shownAvailable)
		}
	}
}

func TestPickerKeysAreTheOnesItAdvertises(t *testing.T) {
	m := openPresets(t, New(testState(t, nil)))

	help := m.View()
	for _, k := range []string{"j/k choose", "enter apply", "esc back", "</> width", "n payload"} {
		if !strings.Contains(help, k) {
			t.Errorf("the picker's help line does not mention %q", k)
		}
	}

	narrow := press(m, "<", "<")
	if narrow.width >= m.width {
		t.Error("the width slider is dead in the picker")
	}
	if narrow.mode != modePresets {
		t.Error("the slider closed the picker")
	}
	if press(m, "n").sourceIdx == m.sourceIdx {
		t.Error("the payload cycler is dead in the picker")
	}

	back := press(m, "q")
	if back.Quit() {
		t.Error("q inside the picker quit the wizard")
	}
	if back.mode != modeSegments {
		t.Error("q did not close the picker")
	}
}

func TestBothFramesCloseOnTheSameRow(t *testing.T) {
	for _, term := range []int{100, 120, 160} {
		for _, picker := range []bool{false, true} {
			st := testState(t, nil)
			st.Columns = 80
			m := New(st)
			m.term = term
			if picker {
				m = openPresets(t, m)
			}
			if !m.sideBySide() {
				continue
			}

			last := ""
			for _, l := range strings.Split(m.View(), "\n") {
				if strings.Contains(l, "╰") {
					last = l
				}
			}
			if got := strings.Count(last, "╰"); got != 2 {
				t.Errorf("term %d (picker %v): the last row to close carries %d corners, "+
					"want both panes' — %q", term, picker, got, last)
			}
		}
	}
}

func TestEveryDocumentedKeyDoesSomething(t *testing.T) {
	base := press(New(testState(t, (&saveRecorder{}).fn)), "j", "j")
	if d := base.rows[0].Segments[2].Drop; d == 0 || d == config.NeverDrop {
		t.Fatalf("the fixture segment sits at drop %d, so +/- cannot both move", d)
	}
	help := base.View()

	for _, k := range []string{"j", "k", "J", "K", "space", "+", "-", "<", ">",
		"i", "p", "c", "n", "r", "s", "ctrl+s"} {
		if got := press(base, k); reflect.DeepEqual(snapshot(got), snapshot(base)) {
			t.Errorf("%q changed nothing", k)
		}
	}
	for _, k := range []string{"j/k", "J/K", "space", "+/-", "</>", "i icons",
		"p powerline", "c colour", "n payload", "r reset", "s save",
		"ctrl+s save & apply", "q quit"} {
		if !strings.Contains(help, k) {
			t.Errorf("the help line does not mention %q", k)
		}
	}
}

func snapshot(m Model) string {
	return strings.Join(names(m), ",") + "|" + drops(m) + "|" +
		strings.Join(parkedNames(m), ",") + "|" +
		m.icons + m.powerline + m.colour + m.status + m.err +
		string(rune('0'+m.cursor)) + string(rune('0'+m.sourceIdx)) +
		string(rune(m.width)) + fmt.Sprint(m.mode)
}

func drops(m Model) string {
	var b strings.Builder
	for _, r := range m.rows {
		for _, s := range r.Segments {
			fmt.Fprintf(&b, "%s=%d,", s.Name, s.Drop)
		}
	}
	return b.String()
}

func parkedNames(m Model) []string {
	var out []string
	for _, s := range m.parked {
		out = append(out, s.Name)
	}
	return out
}

func firstRowSlot(m Model, idx int) Model {
	m.seek(slot{0, idx})
	return m
}

func TestFAddsTheMarkerWhereTheGapWillBe(t *testing.T) {
	m := New(testState(t, nil))
	before := len(m.rows[0].Segments)

	m = press(firstRowSlot(m, 0), "f")

	if got := len(m.rows[0].Segments); got != before+1 {
		t.Fatalf("row 1 has %d entries, want %d", got, before+1)
	}
	if !m.rows[0].Segments[1].IsFlex() {
		t.Errorf("the marker did not land after the cursor: %v", names(m))
	}
	if !m.dirty {
		t.Error("adding a marker did not mark the model dirty")
	}
	if s, _ := m.at(); s != (slot{0, 1}) {
		t.Errorf("the cursor is on %+v, want the marker at {0 1}", s)
	}
}

func TestSpaceRemovesAMarkerRatherThanParkingIt(t *testing.T) {
	m := New(testState(t, nil))
	before, parkedBefore := len(m.rows[0].Segments), len(m.parked)

	m = press(firstRowSlot(m, 0), "f", "space")

	if got := len(m.rows[0].Segments); got != before {
		t.Errorf("row 1 has %d entries, want the %d it started with: %v", got, before, names(m))
	}
	if got := len(m.parked); got != parkedBefore {
		t.Errorf("the disabled pool grew to %d; a marker was parked in it", got)
	}
	for _, s := range m.parked {
		if s.IsFlex() {
			t.Fatalf("a marker reached the disabled pool: %+v", m.parked)
		}
	}
}

func TestTheMarkerHasNoPriorityToBump(t *testing.T) {
	m := press(firstRowSlot(New(testState(t, nil)), 0), "f")
	m = press(m, "-", "-", "-", "+")

	if got := m.rows[0].Segments[1].Drop; got != config.NeverDrop {
		t.Errorf("the marker's priority moved to %d; it is pinned at %d", got, config.NeverDrop)
	}
}

func TestFRefusesWhereAMarkerWouldNotSurvive(t *testing.T) {
	base := New(testState(t, nil))

	t.Run("last in a row", func(t *testing.T) {
		m := firstRowSlot(base, len(base.rows[0].Segments)-1)
		before := len(m.rows[0].Segments)
		m = press(m, "f")
		if got := len(m.rows[0].Segments); got != before {
			t.Errorf("a trailing marker was accepted: %v", names(m))
		}
		if m.status == "" {
			t.Error("the refusal was silent")
		}
	})

	t.Run("the disabled pool", func(t *testing.T) {
		m := base
		m.parked = append([]config.SegmentRef(nil), config.SegmentRef{Name: "cost"})
		m.seek(slot{parkedRow, 0})
		before := names(m)
		m = press(m, "f")
		if !reflect.DeepEqual(names(m), before) {
			t.Errorf("f edited a row from the disabled pool: %v", names(m))
		}
	})

	t.Run("the presets button", func(t *testing.T) {
		m := base
		m.seek(slot{presetsRow, 0})
		before := names(m)
		m = press(m, "f")
		if !reflect.DeepEqual(names(m), before) {
			t.Errorf("f edited a row from the button: %v", names(m))
		}
	})
}

func TestAMarkerSurvivesToTheSave(t *testing.T) {
	rec := &saveRecorder{}
	press(firstRowSlot(New(testState(t, rec.fn)), 0), "f", "s")

	if len(rec.calls) != 1 {
		t.Fatalf("the save was not called")
	}
	got := rec.calls[0].Lines[0].Segments
	if len(got) < 2 || !got[1].IsFlex() {
		t.Errorf("the marker did not reach the save: %+v", got)
	}
}

func TestTheMarkerPreviewsAsWhatItRenders(t *testing.T) {
	m := press(firstRowSlot(New(testState(t, nil)), 0), "f")

	_, plain, available := m.Preview()
	if len(plain) == 0 {
		t.Fatal("nothing previewed")
	}
	if w := lipgloss.Width(plain[0]); w != available {
		t.Errorf("the previewed line is %d cells, want the %d the marker fills to:\n%q",
			w, available, plain[0])
	}
}

func TestTheSidebarNamesTheMarker(t *testing.T) {
	m := press(firstRowSlot(New(testState(t, nil)), 0), "f")

	pane := m.segmentPane()
	if !strings.Contains(pane, "flex") {
		t.Errorf("the sidebar does not show the marker:\n%s", pane)
	}
	for _, l := range strings.Split(pane, "\n") {
		if !strings.Contains(l, "flex") {
			continue
		}
		if strings.Contains(l, "drop") || strings.Contains(l, "never") {
			t.Errorf("the marker was given a drop column: %q", l)
		}
	}
}

func TestTheHelpAdvertisesTheMarkerKey(t *testing.T) {
	m := New(testState(t, nil))
	if !strings.Contains(m.help(), "f flex") {
		t.Errorf("the help does not mention f:\n%s", m.help())
	}
}

func TestEverySidebarRowIsTheSameShape(t *testing.T) {
	var m Model
	rows := map[string]string{
		config.FlexName: m.segmentRow(config.SegmentRef{Name: config.FlexName}, false, true),
	}
	for _, n := range config.SegmentNames {
		rows[n] = m.segmentRow(config.SegmentRef{Name: n, Drop: config.DefaultDrop}, false, true)
		rows[n+" (never)"] = m.segmentRow(config.SegmentRef{Name: n, Drop: config.NeverDrop}, false, true)
		rows[n+" (disabled)"] = m.segmentRow(config.SegmentRef{Name: n}, false, false)
	}

	want := markerColumn + nameColumn + 1 + dropColumn
	for name, row := range rows {
		if got := lipgloss.Width(strings.TrimSuffix(row, "\n")); got != want {
			t.Errorf("the %s row is %d cells, want %d — its drop column is out of line",
				name, got, want)
		}
	}
	if inner := segmentPaneWidth - paneStyle.GetHorizontalPadding(); want > inner {
		t.Errorf("a sidebar row is %d cells in a %d-cell pane; the pane will widen "+
			"under the segment list and not under the picker", want, inner)
	}
}

func TestTabWalksTheWholeRingAndComesBack(t *testing.T) {
	for _, name := range config.SegmentNames {
		t.Run(name, func(t *testing.T) {
			m := seek(t, New(testState(t, nil)), name)
			ring := m.ringFor(name)
			start := config.VariantOf(name, m.formats)

			seen := map[string]bool{}
			for range ring {
				m = press(m, "tab")
				seen[fmt.Sprint(config.VariantOf(name, m.formats))] = true
			}
			if len(seen) != len(ring) {
				t.Errorf("%d presses on a %d-entry ring reached %d formats",
					len(ring), len(ring), len(seen))
			}
			if got := config.VariantOf(name, m.formats); !reflect.DeepEqual(got, start) {
				t.Errorf("a full cycle did not close:\n got: %v\nwant: %v", got, start)
			}
		})
	}
}

func TestShiftTabIsTheWayBack(t *testing.T) {
	m := seek(t, New(testState(t, nil)), "ratelimit_5h")
	start := config.VariantOf("ratelimit_5h", m.formats)

	if got := config.VariantOf("ratelimit_5h", press(m, "tab", "shift+tab").formats); !reflect.DeepEqual(got, start) {
		t.Errorf("tab then shift+tab did not return:\n got: %v\nwant: %v", got, start)
	}

	ring := m.ringFor("ratelimit_5h")
	back := press(m, "shift+tab")
	if got := config.IndexOfVariant(ring, "ratelimit_5h", back.formats); got != len(ring)-1 {
		t.Errorf("shift+tab from the first format landed on %d, want the last (%d)",
			got, len(ring)-1)
	}
}

func TestTabDoesNotDiscardAHandWrittenFormat(t *testing.T) {
	const mine = "~{n}~"
	st := testState(t, nil)
	st.Config.Segments.Cost.Format = mine

	m := seek(t, New(st), "cost")
	ring := m.ringFor("cost")
	if len(ring) != len(config.Variants["cost"])+1 {
		t.Fatalf("the ring is %d long; the hand-written format is not in it", len(ring))
	}

	for range ring {
		m = press(m, "tab")
	}
	if got := m.formats.Cost.Format; got != mine {
		t.Errorf("a full cycle came back to %q, want the format the file had: %q", got, mine)
	}
	if got := press(m, "shift+tab", "tab").formats.Cost.Format; got != mine {
		t.Errorf("shift+tab then tab left %q, want %q", got, mine)
	}
}

func TestSavingWritesOnlyTheFormatsThatMoved(t *testing.T) {
	rec := &saveRecorder{}
	m := press(New(testState(t, rec.fn)), "s")
	if got := rec.calls[0].Formats; len(got) != 0 {
		t.Errorf("saving an untouched config wrote %v", got)
	}

	m = press(seek(t, m, "cost"), "tab", "s")
	got := rec.calls[1].Formats
	if len(got) != 1 || got[0].Key != "segments.cost.format" {
		t.Fatalf("after one tab on cost the save carried %v", got)
	}
	if want := m.formats.Cost.Format; got[0].Value != want {
		t.Errorf("the save carried %q; the screen was showing %q", got[0].Value, want)
	}
}

func TestResetRestoresTheFormats(t *testing.T) {
	m := New(testState(t, nil))
	before := m.formats

	m = press(seek(t, m, "ratelimit_5h"), "tab", "tab")
	if m.formats == before {
		t.Fatal("two tabs left the formats where they started; this proves nothing")
	}

	m = press(m, "r")
	if m.formats != before {
		t.Errorf("r left the formats at %v, want %v", m.formats, before)
	}
	if m.dirty {
		t.Error("r left the model dirty")
	}
}

func TestApplyingAPresetKeepsTheFormats(t *testing.T) {
	m := press(seek(t, New(testState(t, nil)), "cost"), "tab")
	cycled := m.formats

	m.cursor = len(m.slots()) - 1
	m = press(m, "enter")
	if got := m.previewConfig().Segments; got != cycled {
		t.Errorf("with the picker open the preview used %v, want %v", got, cycled)
	}

	m = press(m, "down", "enter")
	if m.mode == modePresets {
		t.Fatal("the preset was not applied")
	}
	if m.formats != cycled {
		t.Errorf("applying a preset changed the formats to %v, want %v", m.formats, cycled)
	}
}

func TestTheHintNamesTheFormatOnScreen(t *testing.T) {
	m, _ := resize(New(testState(t, nil)), 160)
	m = seek(t, m, "cost")

	formatRow := func(m Model) string {
		for _, l := range strings.Split(m.hintText(120), "\n") {
			if strings.HasPrefix(l, "  format") {
				return l
			}
		}
		t.Fatalf("the hint has no format row:\n%s", m.hintText(120))
		return ""
	}

	before := formatRow(m)
	m = press(m, "tab")
	after := formatRow(m)

	if before == after {
		t.Fatalf("tab did not change the hint's format row: %q", after)
	}
	if want := m.previewContext().Config.Segments.Cost.Format; !strings.HasSuffix(after, want) {
		t.Errorf("the hint row %q does not end in %q, which is the format the preview used",
			after, want)
	}
}

func TestTabSaysSomethingOnEverySidebarRow(t *testing.T) {
	base := press(New(testState(t, nil)), "f")

	for i, s := range base.slots() {
		m := base
		m.cursor = i
		m = press(m, "tab")

		what := "the presets button"
		if s.row != presetsRow {
			what = m.refAt(s).Name
		}
		changed := m.formats != base.formats
		if !changed && m.status == "" && s.row != presetsRow {
			t.Errorf("tab on %s changed nothing and said nothing", what)
		}
	}
}
