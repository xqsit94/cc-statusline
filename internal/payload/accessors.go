package payload

import (
	"math"
	"path/filepath"
	"time"
)

// Every accessor here returns (value, ok). Segments must be able to tell a
// field that is absent from one that is present and zero: `$0.00` on a fresh
// session and a missing `cost` object are different facts, and only one of them
// should render.

// ModelName reports model.display_name.
func (p *Payload) ModelName() (string, bool) {
	if p.Model == nil {
		return "", false
	}
	return deref(p.Model.DisplayName)
}

// ContextPresent reports whether the context_window object exists at all.
//
// PRD §5.3: the context segment is empty only when the object is absent. A
// present object with a null percentage still renders — as `░░░░░░░░░░ 0%`,
// which is the clean startup state.
func (p *Payload) ContextPresent() bool { return p.ContextWindow != nil }

// PercentExact is `p_exact` from PRD §5.3: the unrounded context percentage,
// preferred source first.
//
// Claude Code's used_percentage was measured in M0 to be
// round(total_input_tokens / context_window_size × 100). We hold both operands,
// so we compute the exact value and let its rounding go — it drives the bar
// fill and the gradient ramp, where a continuous value is strictly better.
// The reported field is the fallback, not the source.
//
// Not clamped: the caller clamps where clamping is meaningful. A value above
// 100 is real information about a payload we did not expect.
func (p *Payload) PercentExact() (float64, bool) {
	if p.ContextWindow == nil {
		return 0, false
	}
	cw := p.ContextWindow

	tokens, hasTokens := deref(cw.TotalInputTokens)
	size, hasSize := deref(cw.ContextWindowSize)
	if hasTokens && hasSize && size > 0 {
		return 100 * tokens / size, true
	}
	if used, ok := deref(cw.UsedPercentage); ok {
		return used, true
	}
	// Object present, nothing usable in it. PRD §5.3: renders as zero.
	return 0, true
}

// PercentShown is `p_shown` from PRD §5.3: what the user reads, and the value
// §5.4's bands compare against so the number and its colour never disagree.
func (p *Payload) PercentShown() (int, bool) {
	exact, ok := p.PercentExact()
	if !ok {
		return 0, false
	}
	return int(clamp(math.Round(exact), 0, 100)), true
}

// WindowSize reports context_window.context_window_size.
func (p *Payload) WindowSize() (float64, bool) {
	if p.ContextWindow == nil {
		return 0, false
	}
	return deref(p.ContextWindow.ContextWindowSize)
}

// CostUSD reports cost.total_cost_usd.
func (p *Payload) CostUSD() (float64, bool) {
	if p.Cost == nil {
		return 0, false
	}
	return deref(p.Cost.TotalCostUSD)
}

// Duration reports cost.total_duration_ms as a duration.
func (p *Payload) Duration() (time.Duration, bool) {
	if p.Cost == nil {
		return 0, false
	}
	ms, ok := deref(p.Cost.TotalDurationMS)
	if !ok {
		return 0, false
	}
	return time.Duration(ms) * time.Millisecond, true
}

// LinesChanged reports the session diffstat. PRD §3.2: this comes from the
// payload, not from git — these are session-scoped counts, which is the more
// useful quantity on a status line than the worktree's total drift.
func (p *Payload) LinesChanged() (added, removed int, ok bool) {
	if p.Cost == nil {
		return 0, 0, false
	}
	a, hasA := deref(p.Cost.TotalLinesAdded)
	r, hasR := deref(p.Cost.TotalLinesRemoved)
	if !hasA && !hasR {
		return 0, 0, false
	}
	return int(a), int(r), true
}

// RateLimitKey selects a rate limit window.
type RateLimitKey int

const (
	FiveHour RateLimitKey = iota
	SevenDay
)

// RateLimitPercent reports one window's used percentage. Either window may be
// absent independently: a non-subscriber has neither, and both are absent
// before the first response of a session.
func (p *Payload) RateLimitPercent(k RateLimitKey) (float64, bool) {
	if p.RateLimits == nil {
		return 0, false
	}
	var w *RateWindow
	switch k {
	case FiveHour:
		w = p.RateLimits.FiveHour
	case SevenDay:
		w = p.RateLimits.SevenDay
	}
	if w == nil {
		return 0, false
	}
	return deref(w.UsedPercentage)
}

// RateLimitResetsAt reports one window's reset time. The payload carries unix
// epoch seconds as a number, not an ISO-8601 string (M0, PRD §3.1).
func (p *Payload) RateLimitResetsAt(k RateLimitKey) (time.Time, bool) {
	if p.RateLimits == nil {
		return time.Time{}, false
	}
	var w *RateWindow
	switch k {
	case FiveHour:
		w = p.RateLimits.FiveHour
	case SevenDay:
		w = p.RateLimits.SevenDay
	}
	if w == nil {
		return time.Time{}, false
	}
	epoch, ok := deref(w.ResetsAt)
	if !ok {
		return time.Time{}, false
	}
	return time.Unix(int64(epoch), 0), true
}

// ProjectName reports the basename of workspace.project_dir.
func (p *Payload) ProjectName() (string, bool) {
	dir, ok := p.ProjectDir()
	if !ok {
		return "", false
	}
	base := filepath.Base(filepath.Clean(dir))
	if base == "." || base == string(filepath.Separator) {
		return "", false
	}
	return base, true
}

// ProjectDir reports workspace.project_dir.
func (p *Payload) ProjectDir() (string, bool) {
	if p.Workspace == nil {
		return "", false
	}
	return deref(p.Workspace.ProjectDir)
}

// CurrentDir reports workspace.current_dir, the input to git discovery
// (PRD §5.8). It is read, never displayed.
func (p *Payload) CurrentDir() (string, bool) {
	if p.Workspace == nil {
		return "", false
	}
	return deref(p.Workspace.CurrentDir)
}

func deref[T any](p *T) (T, bool) {
	if p == nil {
		var zero T
		return zero, false
	}
	return *p, true
}

func clamp(v, lo, hi float64) float64 {
	return math.Min(math.Max(v, lo), hi)
}
