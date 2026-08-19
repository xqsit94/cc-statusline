package line

type Placement int

const (
	Absent Placement = iota
	Shown
	Truncated
	Dropped
)

func (p Placement) String() string {
	switch p {
	case Shown:
		return "shown"
	case Truncated:
		return "truncated"
	case Dropped:
		return "dropped"
	default:
		return "absent"
	}
}

func Trace(ctx Context) [][]Placement {
	out := make([][]Placement, len(ctx.Config.Lines))
	for i, l := range ctx.Config.Lines {
		row := make([]Placement, len(l.Segments))
		_, built, kept := layout(ctx, l)
		for _, it := range built {
			row[it.idx] = Dropped
		}
		for _, it := range kept {
			row[it.idx] = Shown
			if it.truncated {
				row[it.idx] = Truncated
			}
		}
		out[i] = row
	}
	return out
}
