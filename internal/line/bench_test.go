package line

import (
	"testing"

	"github.com/xqsit94/cc-statusline/internal/refstate"
)

func BenchmarkRender(b *testing.B) {
	for _, st := range refstate.References() {
		b.Run(st.Name, func(b *testing.B) {
			ctx := goldenContext(b, st, "unicode", "plain", 120, "1")
			b.ReportAllocs()
			for b.Loop() {
				sink = Render(ctx)
			}
		})
	}
}

func BenchmarkRenderPlain(b *testing.B) {
	ctx := goldenContext(b, refstate.References()[0], "unicode", "plain", 120, "1")
	b.ReportAllocs()
	for b.Loop() {
		sink = RenderPlain(ctx)
	}
}

func BenchmarkFitNarrow(b *testing.B) {
	st, ok := refstate.ByName("danger-92")
	if !ok {
		b.Fatal("danger-92 is gone; this benchmark measured the wrong thing")
	}
	for _, cols := range []int{40, 80} {
		b.Run(itoa(cols), func(b *testing.B) {
			ctx := goldenContext(b, st, "nerdfont", "powerline", cols, "1")
			b.ReportAllocs()
			for b.Loop() {
				sink = RenderPlain(ctx)
			}
		})
	}
}

func BenchmarkRenderWide(b *testing.B) {
	ctx := goldenContext(b, refstate.References()[0], "unicode", "plain", 200, "1")
	b.ReportAllocs()
	for b.Loop() {
		sink = Render(ctx)
	}
}

var sink []string
