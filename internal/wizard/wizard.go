package wizard

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/xqsit94/cc-statusline/internal/config"
	"github.com/xqsit94/cc-statusline/internal/gitinfo"
	"github.com/xqsit94/cc-statusline/internal/line"
	"github.com/xqsit94/cc-statusline/internal/payload"
	"github.com/xqsit94/cc-statusline/internal/style"
)

type Source struct {
	Name    string
	Desc    string
	Payload *payload.Payload
	Git     gitinfo.Info
	Env     map[string]string
}

type Result struct {
	Lines     []config.Line
	Icons     string
	Powerline string
	Colour    string
	Formats   []config.KeyValue
}

type Apply struct {
	Target string
	Do     func() (string, error)
}

type Preset struct {
	Name string
	Desc string
	Result
}

type State struct {
	Config  *config.Config
	Env     map[string]string
	Sources []Source
	Presets []Preset
	Columns int
	Save    func(Result) (string, error)
	Apply   Apply
}

type Model struct {
	rows   []config.Line
	parked []config.SegmentRef

	formats config.Segments

	icons     string
	powerline string
	colour    string

	width       int
	widthPinned bool
	term        int
	termRows    int
	sourceIdx   int
	cursor      int

	mode      mode
	presetIdx int

	state   State
	initial editable
	status  string
	err     string
	dirty   bool
	quit    bool
}

type editable struct {
	rows      []config.Line
	parked    []config.SegmentRef
	formats   config.Segments
	icons     string
	powerline string
	colour    string
}

type mode int

const (
	modeSegments mode = iota
	modePresets
	modeApply
)

var (
	iconCycle      = []string{"ascii", "unicode", "nerdfont"}
	powerlineCycle = []string{"auto", "true", "false"}
	colourCycle    = []string{"auto", "truecolor", "256", "16", "none"}
)

func New(st State) Model {
	m := Model{
		state:     st,
		width:     st.Columns,
		formats:   st.Config.Segments,
		icons:     st.Config.General.Icons,
		powerline: st.Config.General.Powerline.String(),
		colour:    st.Config.General.Color,
	}
	if m.width <= 0 {
		m.width = 80
	}
	m.term = m.width
	m.rows = cloneRows(st.Config.Lines)
	m.parked = missingSegments(m.rows)
	m.initial = m.snapshot()
	return m
}

func (m Model) snapshot() editable {
	return editable{
		rows:    cloneRows(m.rows),
		parked:  append([]config.SegmentRef(nil), m.parked...),
		formats: m.formats,
		icons:   m.icons, powerline: m.powerline, colour: m.colour,
	}
}

func missingSegments(rows []config.Line) []config.SegmentRef {
	present := map[string]bool{}
	for _, r := range rows {
		for _, s := range r.Segments {
			present[s.Name] = true
		}
	}
	var out []config.SegmentRef
	for _, name := range config.SegmentNames {
		if !present[name] {
			out = append(out, config.SegmentRef{Name: name, Drop: config.DefaultDrop})
		}
	}
	return out
}

func cloneRows(rows []config.Line) []config.Line {
	out := make([]config.Line, len(rows))
	for i, r := range rows {
		out[i] = config.Line{Segments: append([]config.SegmentRef(nil), r.Segments...)}
	}
	return out
}

func (m *Model) own() {
	m.rows = cloneRows(m.rows)
	m.parked = append([]config.SegmentRef(nil), m.parked...)
}

func (m Model) Init() tea.Cmd { return nil }

type slot struct {
	row, idx int
}

const (
	parkedRow  = -1
	presetsRow = -2
)

func (m Model) slots() []slot {
	var out []slot
	for r, row := range m.rows {
		for i := range row.Segments {
			out = append(out, slot{r, i})
		}
	}
	for i := range m.parked {
		out = append(out, slot{parkedRow, i})
	}
	if len(m.state.Presets) > 0 {
		out = append(out, slot{presetsRow, 0})
	}
	return out
}

func (m Model) onPresetsButton() bool {
	s, ok := m.at()
	return ok && s.row == presetsRow
}

func (m Model) at() (slot, bool) {
	s := m.slots()
	if len(s) == 0 || m.cursor < 0 || m.cursor >= len(s) {
		return slot{}, false
	}
	return s[m.cursor], true
}

func (m *Model) clampCursor() {
	n := len(m.slots())
	switch {
	case n == 0:
		m.cursor = 0
	case m.cursor < 0:
		m.cursor = 0
	case m.cursor >= n:
		m.cursor = n - 1
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		if sz, ok := msg.(tea.WindowSizeMsg); ok {
			if sz.Width > 0 {
				m.term = sz.Width
			}
			if sz.Height > 0 {
				m.termRows = sz.Height
			}
		}
		return m, nil
	}

	m.status, m.err = "", ""

	key := km.String()
	if m.mode == modePresets {
		return m.updatePresets(key)
	}
	if m.mode == modeApply {
		return m.updateApply(key)
	}
	if m.lens(key) {
		return m, nil
	}

	switch key {
	case "q", "esc", "ctrl+c":
		m.quit = true
		return m, tea.Quit

	case "up", "k":
		m.cursor--
		m.clampCursor()
	case "down", "j":
		m.cursor++
		m.clampCursor()

	case "K", "shift+up":
		m.move(-1)
	case "J", "shift+down":
		m.move(+1)

	case " ", "space", "enter":
		if m.onPresetsButton() {
			m.mode = modePresets
		} else {
			m.toggle()
		}
	case "+", "=", "l", "right":
		if m.onPresetsButton() {
			m.mode = modePresets
		} else {
			m.bumpDrop(+1)
		}
	case "-", "_", "h", "left":
		m.bumpDrop(-1)

	case "tab":
		m.cycleFormat(+1)
	case "shift+tab":
		m.cycleFormat(-1)

	case "f":
		m.insertFlex()

	case "i":
		m.icons = next(iconCycle, m.icons)
		m.dirty = true
	case "p":
		m.powerline = next(powerlineCycle, m.powerline)
		m.dirty = true
	case "c":
		m.colour = next(colourCycle, m.colour)
		m.dirty = true

	case "r":
		m.rows = cloneRows(m.initial.rows)
		m.parked = append([]config.SegmentRef(nil), m.initial.parked...)
		m.formats = m.initial.formats
		m.icons, m.powerline, m.colour = m.initial.icons, m.initial.powerline, m.initial.colour
		m.dirty = false
		m.clampCursor()
		m.status = "reset to the loaded configuration"

	case "s":
		return m.save(), nil
	case "ctrl+s":
		return m.confirmApply(), nil
	}
	return m, nil
}

func (m *Model) lens(key string) bool {
	switch key {
	case "<", ",":
		m.width, m.widthPinned = clamp(m.sliderWidth()-4, 20, 400), true
	case ">", ".":
		m.width, m.widthPinned = clamp(m.sliderWidth()+4, 20, 400), true
	case "n":
		if len(m.state.Sources) == 0 {
			return false
		}
		m.sourceIdx = (m.sourceIdx + 1) % len(m.state.Sources)
	default:
		return false
	}
	return true
}

func (m Model) updatePresets(key string) (tea.Model, tea.Cmd) {
	if m.lens(key) {
		return m, nil
	}
	switch key {
	case "ctrl+c":
		m.quit = true
		return m, tea.Quit

	case "esc", "q", "h", "left":
		m.mode = modeSegments

	case "up", "k":
		m.presetIdx = clamp(m.presetIdx-1, 0, len(m.state.Presets)-1)
	case "down", "j":
		m.presetIdx = clamp(m.presetIdx+1, 0, len(m.state.Presets)-1)

	case " ", "space", "enter", "l", "right":
		m.applyPreset()
	}
	return m, nil
}

func (m Model) highlighted() (Preset, bool) {
	if m.mode != modePresets || len(m.state.Presets) == 0 {
		return Preset{}, false
	}
	return m.state.Presets[m.presetIdx%len(m.state.Presets)], true
}

func (m *Model) applyPreset() {
	p, ok := m.highlighted()
	if !ok {
		return
	}
	m.rows = cloneRows(p.Lines)
	m.parked = missingSegments(m.rows)
	m.icons, m.powerline, m.colour = p.Icons, p.Powerline, p.Colour
	m.dirty = true
	m.mode = modeSegments
	m.cursor = 0
	m.clampCursor()
	m.status = "applied the " + p.Name + " preset — s writes it, r restores what was loaded"
}

func (m Model) confirmApply() Model {
	if m.state.Apply.Do == nil {
		m.err = "there is nowhere to install to — neither CLAUDE_CONFIG_DIR nor HOME is set"
		return m
	}
	m.mode = modeApply
	return m
}

func (m Model) updateApply(key string) (tea.Model, tea.Cmd) {
	if key == "ctrl+c" {
		m.quit = true
		return m, tea.Quit
	}
	m.mode = modeSegments
	if key == "y" || key == "enter" {
		return m.saveAndApply(), nil
	}
	m.status = "cancelled — nothing was written"
	return m, nil
}

func (m Model) saveAndApply() Model {
	m = m.save()
	if m.err != "" {
		return m
	}
	note, err := m.state.Apply.Do()
	if err != nil {
		m.err = err.Error()
		return m
	}
	m.status += "\n" + note
	return m
}

func (m Model) save() Model {
	if m.state.Save == nil {
		m.err = "this configuration cannot be written from here — see the note above"
		return m
	}
	note, err := m.state.Save(m.Result())
	if err != nil {
		m.err = err.Error()
		return m
	}
	m.initial = m.snapshot()
	m.dirty = false
	m.status = note
	return m
}

func (m Model) Result() Result {
	var rows []config.Line
	for _, r := range m.rows {
		if len(r.Segments) > 0 {
			rows = append(rows, config.Line{Segments: append([]config.SegmentRef(nil), r.Segments...)})
		}
	}
	return Result{
		Lines: rows, Icons: m.icons, Powerline: m.powerline, Colour: m.colour,
		Formats: config.Changed(m.formats, m.state.Config.Segments),
	}
}

func (m Model) Quit() bool  { return m.quit }
func (m Model) Dirty() bool { return m.dirty }

func (m *Model) move(d int) {
	s, ok := m.at()
	if !ok {
		return
	}
	if s.row < 0 {
		return
	}
	m.own()
	row := m.rows[s.row]

	var dest slot
	switch {
	case s.idx+d >= 0 && s.idx+d < len(row.Segments):
		row.Segments[s.idx], row.Segments[s.idx+d] = row.Segments[s.idx+d], row.Segments[s.idx]
		dest = slot{s.row, s.idx + d}
	case d < 0 && s.row > 0:
		seg := row.Segments[s.idx]
		m.rows[s.row].Segments = remove(row.Segments, s.idx)
		m.rows[s.row-1].Segments = append(m.rows[s.row-1].Segments, seg)
		dest = slot{s.row - 1, len(m.rows[s.row-1].Segments) - 1}
	case d > 0 && s.row < len(m.rows)-1:
		seg := row.Segments[s.idx]
		m.rows[s.row].Segments = remove(row.Segments, s.idx)
		m.rows[s.row+1].Segments = insert(m.rows[s.row+1].Segments, 0, seg)
		dest = slot{s.row + 1, 0}
	default:
		return
	}
	m.dirty = true
	m.seek(dest)
}

func (m *Model) seek(target slot) {
	for i, s := range m.slots() {
		if s == target {
			m.cursor = i
			break
		}
	}
	m.clampCursor()
}

func (m *Model) toggle() {
	s, ok := m.at()
	if !ok || s.row == presetsRow {
		return
	}
	if s.row >= 0 && m.rows[s.row].Segments[s.idx].IsFlex() {
		m.own()
		m.rows[s.row].Segments = remove(m.rows[s.row].Segments, s.idx)
		m.dirty = true
		m.clampCursor()
		return
	}
	m.own()
	if s.row == parkedRow {
		if len(m.rows) == 0 {
			m.rows = []config.Line{{}}
		}
		last := len(m.rows) - 1
		m.rows[last].Segments = append(m.rows[last].Segments, m.parked[s.idx])
		m.parked = remove(m.parked, s.idx)
	} else {
		seg := m.rows[s.row].Segments[s.idx]
		m.rows[s.row].Segments = remove(m.rows[s.row].Segments, s.idx)
		m.parked = append(m.parked, seg)
		sortByCanonicalOrder(m.parked)
	}
	m.dirty = true
	m.clampCursor()
}

func (m *Model) insertFlex() {
	s, ok := m.at()
	if !ok || s.row < 0 {
		m.status = "f adds a flex gap after a segment — put the cursor on one first"
		return
	}
	if s.idx == len(m.rows[s.row].Segments)-1 {
		m.status = "a flex at the end of a row has nothing to push — add it before a segment"
		return
	}
	m.own()
	m.rows[s.row].Segments = insert(m.rows[s.row].Segments, s.idx+1,
		config.SegmentRef{Name: config.FlexName, Drop: config.NeverDrop})
	m.dirty = true
	m.seek(slot{s.row, s.idx + 1})
	m.status = "flex gap added — everything after it moves right; space removes it"
}

func (m *Model) bumpDrop(d int) {
	s, ok := m.at()
	if !ok || s.row < 0 {
		return
	}
	if m.rows[s.row].Segments[s.idx].IsFlex() {
		return
	}
	m.own()
	seg := &m.rows[s.row].Segments[s.idx]
	seg.Drop = clamp(seg.Drop+d, 0, config.NeverDrop)
	m.dirty = true
}

func (m *Model) cycleFormat(d int) {
	s, ok := m.at()
	if !ok || s.row == presetsRow {
		return
	}
	name := m.refAt(s).Name
	if m.refAt(s).IsFlex() {
		m.status = "a flex gap has no format — it is whatever width the row did not use"
		return
	}
	ring := m.ringFor(name)
	if len(ring) < 2 {
		m.status = oneFormatNote(name)
		return
	}
	cur := max(0, config.IndexOfVariant(ring, name, m.formats))
	i := ((cur+d)%len(ring) + len(ring)) % len(ring)
	config.ApplyVariant(ring[i], &m.formats)
	m.dirty = true
	m.status = fmt.Sprintf("%s format %d of %d — %s", name, i+1, len(ring), leadFormat(ring[i]))
}

func (m Model) ringFor(name string) []config.Variant {
	vs := config.Variants[name]
	if len(vs) == 0 || config.IndexOfVariant(vs, name, m.state.Config.Segments) >= 0 {
		return vs
	}
	return append([]config.Variant{config.VariantOf(name, m.state.Config.Segments)}, vs...)
}

func leadFormat(v config.Variant) string {
	if len(v) == 0 {
		return ""
	}
	return v[0].Value
}

func oneFormatNote(name string) string {
	if name == "branch" {
		return "branch has one format — what labels it is the glyph in front, and i cycles that"
	}
	return name + " has one format"
}

func sortByCanonicalOrder(refs []config.SegmentRef) {
	rank := map[string]int{}
	for i, n := range config.SegmentNames {
		rank[n] = i
	}
	for i := 1; i < len(refs); i++ {
		for j := i; j > 0 && rank[refs[j].Name] < rank[refs[j-1].Name]; j-- {
			refs[j], refs[j-1] = refs[j-1], refs[j]
		}
	}
}

func remove[T any](s []T, i int) []T {
	out := append([]T(nil), s[:i]...)
	return append(out, s[i+1:]...)
}

func insert[T any](s []T, i int, v T) []T {
	out := append([]T(nil), s[:i]...)
	out = append(out, v)
	return append(out, s[i:]...)
}

func clamp(v, lo, hi int) int { return max(lo, min(v, hi)) }

func next(cycle []string, cur string) string {
	for i, v := range cycle {
		if v == cur {
			return cycle[(i+1)%len(cycle)]
		}
	}
	return cycle[0]
}

func (m Model) Preview() (lines []string, plain []string, available int) {
	ctx := m.previewContext()
	return line.Render(ctx), line.RenderPlain(ctx), line.Available(ctx)
}

func (m Model) previewContext() line.Context {
	shown := m.shown()
	cfg := m.configFor(shown)
	src := m.source()

	env := m.state.Env
	if len(src.Env) > 0 {
		env = src.Env
	}
	caps := style.Detect(style.Overlay(env, style.Overrides{
		Icons:   shown.Icons,
		Colour:  colourOverride(shown.Colour),
		Columns: m.sliderWidth(),
	}), cfg)

	return line.Context{Payload: src.Payload, Git: src.Git, Config: cfg,
		Style: style.NewStyle(caps, cfg), Zone: time.Local}
}

func (m Model) sliderWidth() int {
	if m.widthPinned {
		return m.width
	}
	return m.fitWidth()
}

func (m Model) fitWidth() int {
	room := m.term - listWidth(m.leftPane()) - border - paneGap -
		paneStyle.GetHorizontalFrameSize()
	if room < minPaneRule {
		return m.term
	}
	g := m.state.Config.General
	return clamp(room+2*g.Padding+g.WidthReserve, 20, 400)
}

const minPaneRule = 60

func (m Model) shown() Result {
	if p, ok := m.highlighted(); ok {
		return p.Result
	}
	return m.Result()
}

func (m Model) previewConfig() *config.Config { return m.configFor(m.shown()) }

func (m Model) configFor(r Result) *config.Config {
	cfg := *m.state.Config
	cfg.Lines = r.Lines
	cfg.Segments = m.formats
	cfg.General.Icons = r.Icons
	cfg.General.Powerline = config.Flexible(r.Powerline)
	cfg.General.Color = r.Colour
	cfg.General.MaxWidth = 0
	return &cfg
}

func colourOverride(v string) string {
	if v == "auto" {
		return ""
	}
	return v
}

func (m Model) source() Source {
	if len(m.state.Sources) == 0 {
		return Source{Name: "none"}
	}
	return m.state.Sources[m.sourceIdx%len(m.state.Sources)]
}

func (m Model) NoColorIsSet() bool {
	env := m.state.Env
	if src := m.source(); len(src.Env) > 0 {
		env = src.Env
	}
	_, ok := env["NO_COLOR"]
	return ok
}

var (
	borderColour = lipgloss.AdaptiveColor{Light: "#b4b4b4", Dark: "#3f3f46"}

	paneStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderColour).
			Padding(0, 1)

	hintStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderColour).
			Padding(0, 1)

	helpStyle = lipgloss.NewStyle().
			Faint(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(borderColour).
			BorderTop(true).
			Padding(0, 1)

	headingStyle = lipgloss.NewStyle().Bold(true)
	cursorStyle  = lipgloss.NewStyle().Bold(true)
	dimStyle     = lipgloss.NewStyle().Faint(true)
	warningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#facc15"))
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#ef4444"))
)

const flexLabel = "‹  flex  ›"

const dropColumn = 7

var nameColumn = func() int {
	w := lipgloss.Width(flexLabel)
	for _, n := range config.SegmentNames {
		w = max(w, lipgloss.Width(n))
	}
	return w + 1
}()

const markerColumn = 2

var segmentPaneWidth = markerColumn + nameColumn + 1 + dropColumn +
	paneStyle.GetHorizontalPadding()

func inNameColumn(s string) string {
	return s + strings.Repeat(" ", max(0, nameColumn-lipgloss.Width(s)))
}

func inDropColumn(s string) string {
	return strings.Repeat(" ", max(0, dropColumn-lipgloss.Width(s))) + s
}

const paneGap = 1

var border = paneStyle.GetHorizontalBorderSize()

func (m Model) View() string {
	full := m.view(true)
	if m.termRows <= 0 || lipgloss.Height(full) <= m.termRows {
		return full
	}
	return m.view(false)
}

func (m Model) view(hint bool) string {
	left := m.leftPane()

	var body string
	if m.sideBySide() {
		listBox := paneStyle.Width(listWidth(left))
		w := m.previewWidth(lipgloss.Width(listBox.Render(left)) + paneGap)
		right := m.previewPane(m.hintBox(hint, w))
		previewBox := paneStyle.Width(w)

		h := max(lipgloss.Height(listBox.Render(left)), lipgloss.Height(previewBox.Render(right))) -
			paneStyle.GetVerticalBorderSize()
		body = lipgloss.JoinHorizontal(lipgloss.Top,
			listBox.Height(h).MarginRight(paneGap).Render(left),
			previewBox.Height(h).Render(right))
	} else {
		w := m.previewWidth(0)
		body = lipgloss.JoinVertical(lipgloss.Left,
			paneStyle.Width(listWidth(left)).Render(left),
			paneStyle.Width(w).Render(m.previewPane(m.hintBox(hint, w))))
	}

	var b strings.Builder
	b.WriteString(body)
	if m.mode == modeApply {
		b.WriteString("\n" + m.applyBox(lipgloss.Width(body)))
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Width(lipgloss.Width(body)).Render(m.help()))
	if m.mode == modeApply {
		return b.String()
	}
	if m.status != "" {
		b.WriteString(footer(dimStyle, m.status))
	}
	switch {
	case m.err != "":
		b.WriteString(footer(errorStyle, m.err))
	case m.status == "" && m.dirty:
		b.WriteString(footer(warningStyle, "unsaved changes — s writes, q discards"))
	}
	return b.String()
}

func footer(st lipgloss.Style, s string) string {
	var b strings.Builder
	for _, l := range strings.Split(s, "\n") {
		b.WriteString("\n " + st.Render(l))
	}
	return b.String()
}

func (m Model) applyBox(w int) string {
	var b strings.Builder
	b.WriteString(headingStyle.Render("save and apply") + "\n\n")
	b.WriteString("Write your configuration, then install this status line into\n")
	b.WriteString("  " + m.state.Apply.Target + "\n")
	b.WriteString("so that Claude Code runs it. That file is backed up first, and\n")
	b.WriteString("the only key in it that changes is \"statusLine\".")
	return hintStyle.Width(max(w-hintStyle.GetHorizontalBorderSize(), 0)).Render(b.String())
}

func (m Model) leftPane() string {
	if m.mode == modePresets {
		return m.presetPane()
	}
	return m.segmentPane()
}

func (m Model) help() string {
	switch m.mode {
	case modePresets:
		return "j/k choose   enter apply   esc back\n" +
			"</> width   n payload\n" +
			"applying is an edit — nothing is written until you save"
	case modeApply:
		return "y or enter   save and apply\n" +
			"n or esc     cancel\n" +
			"nothing has been written yet"
	}
	return "j/k move   J/K reorder   space on/off   +/- drop   tab format   f flex gap\n" +
		"</> width   n payload   i icons   p powerline   c colour\n" +
		"r reset   s save   ctrl+s save & apply   q quit"
}

func (m Model) sideBySide() bool {
	_, _, available := m.Preview()
	list := listWidth(m.leftPane()) + border
	preview := available + paneStyle.GetHorizontalFrameSize()
	return m.term >= list+paneGap+preview
}

func (m Model) previewWidth(used int) int {
	_, _, available := m.Preview()
	return max(m.term-used-border, available+paneStyle.GetHorizontalPadding())
}

func listWidth(left string) int {
	return max(segmentPaneWidth, lipgloss.Width(left)+paneStyle.GetHorizontalPadding())
}

func (m Model) segmentPane() string {
	var b strings.Builder
	b.WriteString(headingStyle.Render("segments") + "\n")

	cur, hasCur := m.at()
	for r, row := range m.rows {
		fmt.Fprintf(&b, "\n%s\n", dimStyle.Render(fmt.Sprintf("line %d", r+1)))
		if len(row.Segments) == 0 {
			b.WriteString(dimStyle.Render("  (empty — this row will not be written)") + "\n")
		}
		for i, s := range row.Segments {
			b.WriteString(m.segmentRow(s, hasCur && cur.row == r && cur.idx == i, true))
		}
	}

	b.WriteString("\n" + dimStyle.Render("disabled") + "\n")
	if len(m.parked) == 0 {
		b.WriteString(dimStyle.Render("  (none)") + "\n")
	}
	for i, s := range m.parked {
		b.WriteString(m.segmentRow(s, hasCur && cur.row == parkedRow && cur.idx == i, false))
	}

	if len(m.state.Presets) > 0 {
		b.WriteString("\n" + button("Presets ›", hasCur && cur.row == presetsRow) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func button(label string, selected bool) string {
	if selected {
		return cursorStyle.Render("▸ " + label)
	}
	return "  " + label
}

func (m Model) segmentRow(s config.SegmentRef, selected, enabled bool) string {
	marker := "  "
	if selected {
		marker = "▸ "
	}
	if s.IsFlex() {
		row := marker + inNameColumn(flexLabel) + " " + inDropColumn("")
		if selected {
			return cursorStyle.Render(row) + "\n"
		}
		return dimStyle.Render(row) + "\n"
	}
	drop := fmt.Sprintf("drop %2d", s.Drop)
	if s.Drop == config.NeverDrop {
		drop = "never"
	}
	if !enabled {
		return dimStyle.Render(marker+inNameColumn(s.Name)+" "+inDropColumn("—")) + "\n"
	}
	row := marker + inNameColumn(s.Name) + " " + inDropColumn(drop)
	if selected {
		row = cursorStyle.Render(row)
	}
	return row + "\n"
}

func (m Model) presetPane() string {
	var b strings.Builder
	b.WriteString(headingStyle.Render("presets") + "\n\n")
	for i, p := range m.state.Presets {
		row := fmt.Sprintf("%s%s %*s", "  ", inNameColumn(p.Name), dropColumn, shape(p))
		if i == m.presetIdx {
			row = cursorStyle.Render(fmt.Sprintf("%s%s %*s", "▸ ",
				inNameColumn(p.Name), dropColumn, shape(p)))
		}
		b.WriteString(row + "\n")
	}
	b.WriteString("\n" + dimStyle.Render("  ‹ back") + "\n")
	return strings.TrimRight(b.String(), "\n")
}

func shape(p Preset) string {
	if len(p.Lines) == 1 {
		return "1 line"
	}
	return fmt.Sprintf("%d lines", len(p.Lines))
}

func (m Model) previewPane(hint string) string {
	rendered, _, available := m.Preview()
	preset, previewingPreset := m.highlighted()

	var b strings.Builder
	if previewingPreset {
		b.WriteString(headingStyle.Render("preview · "+preset.Name) + "\n")
		b.WriteString(dimStyle.Render(preset.Desc) + "\n\n")
	} else {
		b.WriteString(headingStyle.Render("preview") + "\n\n")
	}
	if hint != "" {
		b.WriteString(hint + "\n\n")
	}
	for _, l := range rendered {
		b.WriteString(l + "\n")
	}
	if len(rendered) == 0 {
		b.WriteString(dimStyle.Render("(every row is empty)") + "\n")
	}
	b.WriteString("\n" + dimStyle.Render(style.Rule(available)) + "\n")

	shown, src := m.shown(), m.source()
	fmt.Fprintf(&b, "\n%s\n", dimStyle.Render(fmt.Sprintf(
		"width %d   payload %s   icons %s   powerline %s   colour %s",
		m.sliderWidth(), src.Name, shown.Icons, shown.Powerline, shown.Colour)))
	if previewingPreset {
		b.WriteString(dimStyle.Render(
			"enter applies these rows and the icons/powerline/colour keys;\n"+
				"your colours, formats and thresholds are kept") + "\n")
	}
	if m.NoColorIsSet() {
		b.WriteString(warningStyle.Render(
			"NO_COLOR is set in your environment; it wins over this setting (§6.3)") + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
