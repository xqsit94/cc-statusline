package spike

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/xqsit94/cc-statusline/internal/payload"
)

// expectedKeys is the payload contract as docs/PRD.md §3.1 asserts it — both
// the rendered fields and the "available but not rendered" list. Every path
// here is an assumption that has never been run once. The report's job is to
// mark each one confirmed or refuted.
//
// Paths ending in ".*" are prefixes: any observed key beneath them counts as
// expected, because §3.1 names the subtree without naming its members.
var expectedKeys = []string{
	"model.display_name",
	"model.id",
	"context_window.used_percentage",
	"context_window.remaining_percentage",
	"context_window.context_window_size",
	"context_window.total_input_tokens",
	"context_window.total_output_tokens",
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

type keyStat struct {
	seen   int
	kinds  map[string]bool
	sample string
}

// window is one distinct observation of the context accounting.
type window struct {
	size      float64
	used      float64
	hasUsed   bool
	remaining float64
	hasRem    bool
	occ       float64
	hasOcc    bool
	parts     string
	count     int
}

// Report implements `cc-statusline report`.
func Report(w io.Writer) int {
	dir, err := SpoolDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "cc-statusline:", err)
		return 1
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cc-statusline: no spooled payloads (%v)\n", err)
		fmt.Fprintln(os.Stderr, "Run `cc-statusline capture` as your status line first.")
		return 1
	}

	var docs []doc
	var raws [][]byte
	var malformed int
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			malformed++
			continue
		}
		d, ok := parse(b)
		if !ok {
			malformed++
			continue
		}
		docs = append(docs, d)
		raws = append(raws, b)
	}

	if len(docs) == 0 {
		fmt.Fprintf(os.Stderr, "cc-statusline: %s holds no parseable payloads\n", dir)
		return 1
	}

	fmt.Fprintf(w, "cc-statusline — M0 report (docs/PRD.md §3.1.1)\n\n")
	fmt.Fprintf(w, "spool      %s\n", dir)
	fmt.Fprintf(w, "payloads   %d parsed", len(docs))
	if malformed > 0 {
		fmt.Fprintf(w, ", %d unreadable", malformed)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "sessions   %s\n", strings.Join(distinct(docs, "session_id"), ", "))
	fmt.Fprintf(w, "versions   %s\n", strings.Join(distinct(docs, "version"), ", "))
	fmt.Fprintf(w, "models     %s\n", strings.Join(distinct(docs, "model.display_name"), ", "))

	reportContract(w, docs, raws)
	reportDenominator(w, docs)
	return 0
}

// ── Question 1: do the fields exist with the shapes claimed? ───────────────

func reportContract(w io.Writer, docs []doc, raws [][]byte) {
	stats := map[string]*keyStat{}
	for i, d := range docs {
		// internal/payload owns the flattener now. Running the spike's report
		// through it exercises the real implementation against every payload
		// on disk, which is a better test than any fixture.
		observed, err := payload.FlattenKeys(raws[i])
		if err != nil {
			continue
		}
		for path, kind := range observed {
			st := stats[path]
			if st == nil {
				st = &keyStat{kinds: map[string]bool{}}
				stats[path] = st
			}
			st.seen++
			st.kinds[kind] = true
			if st.sample == "" {
				st.sample = sampleOf(d, path)
			}
		}
	}

	paths := make([]string, 0, len(stats))
	keyWidth := 3
	for p := range stats {
		paths = append(paths, p)
		keyWidth = max(keyWidth, len(p))
	}
	sort.Strings(paths)

	section(w, "1. Payload contract — is §3.1 true? (PRD §3.1.1 Q1)")
	fmt.Fprintf(w, "  %-*s %7s  %-12s %s\n", keyWidth, "KEY", "SEEN", "TYPE", "SAMPLE")
	for _, p := range paths {
		st := stats[p]
		mark := " "
		if !isExpected(p) {
			mark = "+" // observed, undocumented — a segment we could add, or drift
		}
		fmt.Fprintf(w, "%s %-*s %3d/%-3d  %-12s %s\n",
			mark, keyWidth, p, st.seen, len(docs), strings.Join(sortedKeys(st.kinds), "|"), st.sample)
	}

	var missing []string
	for _, want := range expectedKeys {
		if strings.HasSuffix(want, ".*") {
			prefix := strings.TrimSuffix(want, "*")
			found := false
			for p := range stats {
				if strings.HasPrefix(p, prefix) {
					found = true
					break
				}
			}
			if !found {
				missing = append(missing, want)
			}
			continue
		}
		if _, ok := stats[want]; !ok {
			missing = append(missing, want)
		}
	}

	fmt.Fprintln(w)
	if len(missing) == 0 {
		fmt.Fprintln(w, "  ✓ every key §3.1 claims was observed at least once.")
	} else {
		fmt.Fprintln(w, "  ✗ claimed by §3.1, never observed:")
		for _, m := range missing {
			fmt.Fprintf(w, "      %s\n", m)
		}
		fmt.Fprintln(w, "    Absent ≠ refuted: a key can be conditional (subscriber-only,")
		fmt.Fprintln(w, "    post-first-response, vim mode on). Check the condition before")
		fmt.Fprintln(w, "    editing §3.1 — but a key nothing can produce is not a contract.")
	}
	fmt.Fprintln(w, "  Lines marked + are undocumented. Add them to §3.1 or explain them.")
}

// ── Question 2: what is used_percentage a percentage OF? ───────────────────

// tiCrossCheck counts payloads where total_input_tokens equals the summed
// input-side current_usage fields.
func tiCrossCheck(docs []doc) (agree, total int) {
	for _, d := range docs {
		occ, _, ok := d.occupancy()
		ti, tiOK := d.totalInput()
		if !ok || !tiOK {
			continue
		}
		total++
		if math.Abs(occ-ti) < 0.5 {
			agree++
		}
	}
	return agree, total
}

func reportDenominator(w io.Writer, docs []doc) {
	seen := map[string]*window{}
	var order []string

	for _, d := range docs {
		var obs window
		obs.size, _ = d.num("context_window.context_window_size")
		obs.used, obs.hasUsed = d.num("context_window.used_percentage")
		obs.remaining, obs.hasRem = d.num("context_window.remaining_percentage")
		var parts []string
		obs.occ, parts, obs.hasOcc = d.occupancy()
		obs.parts = strings.Join(parts, " ")

		key := fmt.Sprintf("%.0f|%.4f|%v|%.0f|%v", obs.size, obs.used, obs.hasUsed, obs.occ, obs.hasOcc)
		if prev, ok := seen[key]; ok {
			prev.count++
			continue
		}
		obs.count = 1
		cp := obs
		seen[key] = &cp
		order = append(order, key)
	}

	rows := make([]*window, 0, len(order))
	for _, k := range order {
		rows = append(rows, seen[k])
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].used < rows[j].used })

	section(w, "2. What is used_percentage a percentage OF? (PRD §3.1.1 Q2)")

	// Every observed used_percentage being integral changes the whole method.
	// A single implied denominator (occupancy ÷ used) is then meaningless: at
	// used=10 a ±0.5 rounding error moves the implied window by ±5%, which
	// reads as wild inconsistency when the data is in fact perfectly regular.
	// So: derive a feasible RANGE per observation and intersect the ranges.
	integral := true
	for _, r := range rows {
		if r.hasUsed && math.Abs(r.used-math.Round(r.used)) > 1e-9 {
			integral = false
		}
	}
	if integral {
		fmt.Fprintln(w, "  used_percentage is integer-valued in every observation. Treating it")
		fmt.Fprintln(w, "  as rounded, so each row constrains the denominator to a range rather")
		fmt.Fprintln(w, "  than fixing it to a point.")
		fmt.Fprintln(w)
	}

	fmt.Fprintf(w, "  %-4s %9s %7s %8s %10s %8s %9s %6s %19s\n",
		"N", "WINDOW", "USED%", "REMAIN%", "OCCUPANCY", "RAW%", "round()", "MATCH", "FEASIBLE DENOMINATOR")

	loBound, hiBound := math.Inf(-1), math.Inf(1)
	matches, checks := 0, 0

	for _, r := range rows {
		usedS, remS, occS, rawS, roundS, matchS, rangeS := "—", "—", "—", "—", "—", "—", "—"
		if r.hasUsed {
			usedS = fmt.Sprintf("%.2f", r.used)
		}
		if r.hasRem {
			remS = fmt.Sprintf("%.2f", r.remaining)
		}
		if r.hasOcc {
			occS = fmt.Sprintf("%.0f", r.occ)
			if r.size > 0 {
				raw := r.occ / r.size * 100
				rawS = fmt.Sprintf("%.2f", raw)
				if r.hasUsed {
					roundS = fmt.Sprintf("%.0f", math.Round(raw))
					checks++
					if math.Round(raw) == math.Round(r.used) {
						matches++
						matchS = "yes"
					} else {
						matchS = "NO"
					}
				}
			}
			// Feasible denominators: values of D for which occupancy/D would
			// round to the reported percentage. Half-width is 0.5 if the value
			// is rounded, and effectively 0 if it is exact.
			if r.hasUsed && r.used > 0 {
				half := 0.5
				if !integral {
					half = 0.005
				}
				lo := r.occ / ((r.used + half) / 100)
				hi := r.occ / ((r.used - half) / 100)
				rangeS = fmt.Sprintf("%.0f–%.0f", lo, hi)
				// Intersect in ratio space, not token space: sessions with
				// different window sizes are otherwise not comparable.
				if r.size > 0 {
					loBound = math.Max(loBound, lo/r.size)
					hiBound = math.Min(hiBound, hi/r.size)
				}
			}
		}
		fmt.Fprintf(w, "  %-4d %9.0f %7s %8s %10s %8s %9s %6s %19s\n",
			r.count, r.size, usedS, remS, occS, rawS, roundS, matchS, rangeS)
	}

	// Show the fullest window observed, not the first: an early-session row is
	// mostly zeros and tells you nothing about the shape of current_usage.
	fullest := 0
	for i, r := range rows {
		if r.occ > rows[fullest].occ {
			fullest = i
		}
	}
	if len(rows) > 0 && rows[fullest].parts != "" {
		fmt.Fprintf(w, "\n  OCCUPANCY sums: %s\n", rows[fullest].parts)
	}
	if agree, total := tiCrossCheck(docs); total > 0 {
		fmt.Fprintf(w, "  total_input_tokens equals that sum in %d/%d payloads", agree, total)
		if agree == total {
			fmt.Fprintln(w, " — one field is enough.")
		} else {
			fmt.Fprintln(w, " — they have diverged; the accounting changed.")
		}
	}

	fmt.Fprintln(w)
	verdict(w, loBound, hiBound, matches, checks, integral, rows)
}

// verdict answers PRD §3.1.1 Q2 from two independent angles: whether the
// reported percentage reproduces from occupancy ÷ context_window_size, and what
// range of denominators the observations actually permit.
func verdict(w io.Writer, lo, hi float64, matches, checks int, integral bool, rows []*window) {
	if checks == 0 || math.IsInf(lo, 0) || math.IsInf(hi, 0) {
		fmt.Fprintln(w, "  VERDICT: insufficient data.")
		fmt.Fprintln(w, "  No observation carried used_percentage, context_window_size and")
		fmt.Fprintln(w, "  current_usage together. Keep capturing — M0 is not done, and")
		fmt.Fprintln(w, "  §5.4's thresholds remain unverified.")
		return
	}

	if lo > hi {
		fmt.Fprintln(w, "  VERDICT: no single denominator explains every observation.")
		fmt.Fprintln(w, "  The feasible ranges above do not intersect, so used_percentage is")
		fmt.Fprintln(w, "  not occupancy divided by any fixed number. Either OCCUPANCY is the")
		fmt.Fprintln(w, "  wrong numerator (see the current_usage note) or the denominator")
		fmt.Fprintln(w, "  moves with session state. Read the rows; do not set §5.4 off this.")
		return
	}

	fmt.Fprintf(w, "  reproduced from occupancy ÷ context_window_size: %d/%d observations\n", matches, checks)
	fmt.Fprintf(w, "  feasible denominator: %.1f%%–%.1f%% of context_window_size\n", lo*100, hi*100)
	if integral {
		fmt.Fprintln(w, "  (the range is wide because used_percentage is rounded to a whole")
		fmt.Fprintln(w, "  number — a long session at high context narrows it sharply)")
	}
	fmt.Fprintln(w)

	// The question is not whether the denominator is exactly
	// context_window_size — it is whether a large deduction has been taken out
	// of it. An autocompact threshold is tens of percent; anything within a few
	// percent of the full window means no threshold is baked in, and the
	// residual is a numerator-definition detail on our side.
	band := (lo + hi) / 2

	switch {
	case lo > 0.95:
		fmt.Fprintln(w, "  VERDICT: used_percentage is a percentage of the RAW context window.")
		fmt.Fprintln(w, "  The feasible denominator sits within a few percent of the full")
		fmt.Fprintln(w, "  window, which rules out an autocompact threshold — a threshold")
		fmt.Fprintln(w, "  would pull the denominator down by tens of percent, not by one.")
		fmt.Fprintln(w, "  So the number does NOT account for compaction.")
		if math.Abs(band-1) > 0.005 {
			size := modalSize(rows)
			fmt.Fprintf(w, "\n  The band centres on %.1f%% rather than 100%%. On a %.0f-token window\n",
				band*100, size)
			fmt.Fprintf(w, "  that is ~%.0f tokens — the gap between the current_usage fields this\n",
				math.Abs(band-1)*size)
			fmt.Fprintln(w, "  report sums and whatever Claude Code actually counts. Worth pinning")
			fmt.Fprintln(w, "  down before §5.3 renders a token figure, but it is a numerator")
			fmt.Fprintln(w, "  definition question, not a threshold.")
		}
		if matches < checks {
			fmt.Fprintf(w, "\n  %d of %d rows disagree with round(RAW%%) — those are the useful\n",
				checks-matches, checks)
			fmt.Fprintln(w, "  ones. A row that straddles a rounding boundary is what narrows the")
			fmt.Fprintln(w, "  feasible band; rows that agree only confirm it is not far off.")
		}
		fmt.Fprintln(w)
		fmt.Fprintln(w, "  Consequences for the spec:")
		fmt.Fprintln(w, "   • Compaction fires before this reaches 100. §5.4's danger=85 may sit")
		fmt.Fprintln(w, "     AFTER the compaction point, making the bar's most important state")
		fmt.Fprintln(w, "     unreachable — §1's first stated purpose, miscalibrated.")
		fmt.Fprintln(w, "   • The remaining M0 measurement is the compaction point itself. Run a")
		fmt.Fprintln(w, "     session into a real /compact and record the used_percentage it")
		fmt.Fprintln(w, "     fired at. That number, not 100, is the scale §5.4 derives from.")
		fmt.Fprintln(w, "   • Rounding is Claude Code's, not ours: §5.5's bar can use the exact")
		fmt.Fprintln(w, "     ratio from current_usage instead and avoid inheriting the step.")
	case hi < 0.95:
		fmt.Fprintf(w, "  VERDICT: the denominator is at most %.1f%% of context_window_size.\n", hi*100)
		fmt.Fprintln(w, "  Part of the window is excluded — consistent with used_percentage")
		fmt.Fprintln(w, "  already being net of the autocompact threshold. If so, 100% means")
		fmt.Fprintln(w, "  compaction, §5.4's thresholds are on the right scale, and §5.3's")
		fmt.Fprintln(w, "  size marker must NOT be computed as used_percentage ×")
		fmt.Fprintln(w, "  context_window_size — that product is not a token count.")
	default:
		fmt.Fprintln(w, "  VERDICT: undecided — the raw window is inside the feasible range but")
		fmt.Fprintln(w, "  so is a substantially smaller denominator. Rounding at low context")
		fmt.Fprintln(w, "  cannot separate them. Capture a session above ~50% context and")
		fmt.Fprintln(w, "  re-run; the range narrows in proportion to the percentage.")
	}

	// The cheapest independent check available: if used + remaining ≠ 100,
	// they are not two views of one denominator and the whole model is wrong.
	for _, r := range rows {
		if r.hasUsed && r.hasRem && math.Abs(r.used+r.remaining-100) > 0.01 {
			fmt.Fprintf(w, "\n  ! used_percentage + remaining_percentage = %.2f, not 100 "+
				"(window %.0f).\n    They do not share a denominator. Re-read §3.1 before M2.\n",
				r.used+r.remaining, r.size)
			break
		}
	}
}

// ── helpers ────────────────────────────────────────────────────────────────

func sampleOf(d doc, path string) string {
	v, ok := d.at(strings.ReplaceAll(path, "[]", ""))
	if !ok {
		return "null"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "?"
	}
	s := string(b)
	if len(s) > 44 {
		s = s[:41] + "..."
	}
	return s
}

func isExpected(path string) bool {
	base := strings.ReplaceAll(path, "[]", "")
	for _, want := range expectedKeys {
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

func distinct(docs []doc, path string) []string {
	set := map[string]bool{}
	for _, d := range docs {
		if s, ok := d.str(path); ok {
			set[s] = true
		}
	}
	if len(set) == 0 {
		return []string{"(none observed)"}
	}
	return sortedKeys(set)
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// modalSize returns the most frequently observed context_window_size, so the
// token figures in the verdict are quoted against the window most of the data
// came from rather than an outlier session.
func modalSize(rows []*window) float64 {
	counts := map[float64]int{}
	for _, r := range rows {
		counts[r.size] += r.count
	}
	var best float64
	for size, n := range counts {
		if n > counts[best] || (n == counts[best] && size > best) {
			best = size
		}
	}
	return best
}

func section(w io.Writer, title string) {
	fmt.Fprintf(w, "\n── %s %s\n\n", title, strings.Repeat("─", max(0, 72-len([]rune(title)))))
}
