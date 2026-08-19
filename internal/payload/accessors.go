package payload

import (
	"math"
	"path/filepath"
	"time"
)

func (p *Payload) ModelName() (string, bool) {
	if p.Model == nil {
		return "", false
	}
	return deref(p.Model.DisplayName)
}

func (p *Payload) ContextPresent() bool { return p.ContextWindow != nil }

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
	return 0, true
}

func (p *Payload) PercentShown() (int, bool) {
	exact, ok := p.PercentExact()
	if !ok {
		return 0, false
	}
	return int(clamp(math.Round(exact), 0, 100)), true
}

func (p *Payload) WindowSize() (float64, bool) {
	if p.ContextWindow == nil {
		return 0, false
	}
	return deref(p.ContextWindow.ContextWindowSize)
}

func (p *Payload) CostUSD() (float64, bool) {
	if p.Cost == nil {
		return 0, false
	}
	return deref(p.Cost.TotalCostUSD)
}

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

type RateLimitKey int

const (
	FiveHour RateLimitKey = iota
	SevenDay
)

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

func (p *Payload) EffortLevel() (string, bool) {
	if p.Effort == nil {
		return "", false
	}
	return deref(p.Effort.Level)
}

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

func (p *Payload) ProjectDir() (string, bool) {
	if p.Workspace == nil {
		return "", false
	}
	return deref(p.Workspace.ProjectDir)
}

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
