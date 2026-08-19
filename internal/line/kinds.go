package line

import (
	"math"
	"strconv"
)

func percent(v float64) int { return int(math.Round(v)) }

func money(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) }
