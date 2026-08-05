package line

import (
	"sort"

	"github.com/xqsit94/cc-statusline/internal/config"
)

// PRD §5.6's fitter, in three escalating stages: drop, truncate, clip.
//
// Stage 3 is what makes never-wrap a guarantee rather than an aspiration. A
// status line that wraps costs the user a terminal row on every prompt and
// makes Claude Code's own notifications overlap it, so "it usually fits" is not
// a property worth having.

// fitted is one rendered segment plus what the fitter needs to reason about it.
type fitted struct {
	ref config.SegmentRef
	seg Segment
	r   Rendered
}

// available is §5.6's budget: `max(20, COLUMNS - 2×padding - width_reserve)`.
//
// The floor of 20 is a floor on absurdity, not on correctness — below it, stage
// 3 clips a line to something unreadable either way, and a negative budget
// would clip every line to nothing at all.
func available(ctx Context) int {
	cols := ctx.Style.Caps.Columns
	return max(20, cols-2*ctx.Config.General.Padding-ctx.Config.General.WidthReserve)
}

// Available exports the budget for `cc-statusline preview`, which prints it
// beside the rendered lines so that §9.4's gate shows the number a line was
// fitted against rather than only the result. M7's width slider needs the same
// thing. Nothing in the render path calls it.
func Available(ctx Context) int { return available(ctx) }

// fit renders one line's segments into a row that occupies at most avail cells.
func fit(ctx Context, items []fitted, avail int) Rendered {
	sep := separator(ctx)
	width := func(r Rendered) int { return ctx.Style.Width(r.Plain) }

	out := joinFitted(items, sep)
	if width(out) <= avail {
		return out
	}

	// ── Stage 1: drop ──────────────────────────────────────────────────────
	// Priorities reflect §1's information hierarchy, not visual weight: the
	// duration dies first because it is the least actionable thing on the line,
	// and rate limits die last among the droppable because they are §1's third
	// question.
	for width(out) > avail {
		i := dropCandidate(items)
		if i < 0 {
			break
		}
		items = append(items[:i:i], items[i+1:]...)
		out = joinFitted(items, sep)
	}
	if width(out) <= avail {
		return out
	}

	// ── Stage 2: truncate ──────────────────────────────────────────────────
	// Only segments that survived stage 1 are here, which in the default preset
	// means the ones marked never-drop. They shrink instead of disappearing:
	// the bar gives up cells, long branch and model names take an ellipsis.
	for _, i := range truncationOrder(items) {
		over := width(out) - avail
		if over <= 0 {
			break
		}
		t, ok := items[i].seg.(Truncatable)
		if !ok {
			continue
		}
		before := width(items[i].r)
		shrunk := t.Truncate(ctx, before-over)
		// A segment may refuse — it has hit its floor, or shrinking would empty
		// it. Refusing is legitimate, and stage 3 is why it can be.
		if shrunk.Empty() || width(shrunk) >= before {
			continue
		}
		items[i].r = shrunk
		out = joinFitted(items, sep)
	}
	if width(out) <= avail {
		return out
	}

	// ── Stage 3: clip ──────────────────────────────────────────────────────
	// Plain and Styled are clipped by different functions because they are
	// different problems: Plain is text, Styled has escape sequences that must
	// be stepped over rather than counted, and needs a reset appended so a cut
	// that lands mid-colour does not paint the rest of the terminal row.
	return Rendered{
		Styled: ctx.Style.ClipStyled(out.Styled, avail),
		Plain:  ctx.Style.TruncateCells(out.Plain, avail),
	}
}

// dropCandidate returns the index to drop next, or -1 when nothing may go.
//
// Highest drop value first; ties break rightmost-first, which the `>=` below
// implements by letting a later equal candidate replace an earlier one. Dropping
// from the right keeps the leftmost of two equally-ranked segments, and the left
// of a line is what the eye reaches first.
func dropCandidate(items []fitted) int {
	best := -1
	for i, it := range items {
		if it.ref.Drop >= config.NeverDrop {
			continue
		}
		if best < 0 || it.ref.Drop >= items[best].ref.Drop {
			best = i
		}
	}
	return best
}

// truncationOrder lists indices in the order stage 2 should ask them:
// ascending drop, ties rightmost-first.
//
// §5.6 states the tie-break for stage 1 only. Extending the same rule to stage 2
// is not merely consistency — it is also the better outcome on the default
// preset, where `model` and `context` are both marked never-drop. Rightmost-first
// asks `context` before `model`, so the bar gives up cells before the model name
// starts losing characters. The bar is decoration whose information survives
// intact at any width; the model name is the answer to §1's first question.
func truncationOrder(items []fitted) []int {
	idx := make([]int, len(items))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		ia, ib := items[idx[a]], items[idx[b]]
		if ia.ref.Drop != ib.ref.Drop {
			return ia.ref.Drop < ib.ref.Drop
		}
		return idx[a] > idx[b]
	})
	return idx
}

func joinFitted(items []fitted, sep Rendered) Rendered {
	parts := make([]Rendered, 0, len(items))
	for _, it := range items {
		parts = append(parts, it.r)
	}
	return joinRendered(parts, sep)
}
