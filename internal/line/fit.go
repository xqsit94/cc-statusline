package line

import (
	"sort"
	"strings"

	"github.com/xqsit94/cc-statusline/internal/config"
)

type fitted struct {
	idx       int
	ref       config.SegmentRef
	seg       Segment
	r         Rendered
	flex      bool
	truncated bool
}

func available(ctx Context) int {
	cols := ctx.Style.Caps.Columns
	return max(20, cols-2*ctx.Config.General.Padding-ctx.Config.General.WidthReserve)
}

func Available(ctx Context) int { return available(ctx) }

func fit(ctx Context, items []fitted, avail int) (Rendered, []fitted) {
	sep := separator(ctx)
	width := func(r Rendered) int { return ctx.Style.Width(r.Plain) }

	out := joinFitted(items, sep, 0)
	if width(out) <= avail {
		return flexed(ctx, items, sep, out, avail), trimFlex(items)
	}

	for width(out) > avail {
		i := dropCandidate(items)
		if i < 0 {
			break
		}
		items = append(items[:i:i], items[i+1:]...)
		out = joinFitted(items, sep, 0)
	}
	if width(out) <= avail {
		return flexed(ctx, items, sep, out, avail), trimFlex(items)
	}

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
		if shrunk.Empty() || width(shrunk) >= before {
			continue
		}
		items[i].r = shrunk
		items[i].truncated = true
		out = joinFitted(items, sep, 0)
	}
	if width(out) <= avail {
		return flexed(ctx, items, sep, out, avail), trimFlex(items)
	}

	return Rendered{
		Styled: ctx.Style.ClipStyled(out.Styled, avail),
		Plain:  ctx.Style.TruncateCells(out.Plain, avail),
	}, trimFlex(items)
}

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

func flexed(ctx Context, items []fitted, sep Rendered, out Rendered, avail int) Rendered {
	slack := avail - ctx.Style.Width(out.Plain)
	if slack <= 0 {
		return out
	}
	return joinFitted(items, sep, slack)
}

func joinFitted(items []fitted, sep Rendered, slack int) Rendered {
	items = trimFlex(items)

	flexes := 0
	for _, it := range items {
		if it.flex {
			flexes++
		}
	}
	share, extra := 0, 0
	if flexes > 0 {
		share, extra = slack/flexes, slack%flexes
	}

	var styled, plain strings.Builder
	seen, prevSegment := 0, false
	for i, it := range items {
		if it.flex {
			cells := share
			if seen < extra {
				cells++
			}
			if i > 0 {
				cells++
			}
			seen++
			gap := strings.Repeat(" ", cells)
			styled.WriteString(gap)
			plain.WriteString(gap)
			prevSegment = false
			continue
		}
		if prevSegment {
			styled.WriteString(sep.Styled)
			plain.WriteString(sep.Plain)
		}
		styled.WriteString(it.r.Styled)
		plain.WriteString(it.r.Plain)
		prevSegment = true
	}
	return Rendered{Styled: styled.String(), Plain: plain.String()}
}

func trimFlex(items []fitted) []fitted {
	for len(items) > 0 && items[len(items)-1].flex {
		items = items[:len(items)-1]
	}
	return items
}
