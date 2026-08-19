package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/xqsit94/cc-statusline/internal/refstate"
)

const budget = 20 * time.Millisecond

const samples = 100

func TestRenderProcessBudget(t *testing.T) {
	if raceEnabled {
		t.Skip("under -race the stopwatch is around a fork from an instrumented " +
			"parent; the child binary is uninstrumented and unchanged. race_off_test.go " +
			"has the measurements. `go test ./...` without -race is the gate.")
	}
	bin := buildBinary(t)
	repo := benchRepo(t)

	st, ok := refstate.ByName("danger-92")
	if !ok {
		t.Fatal("danger-92 is gone; the budget was measured against the widest state")
	}
	stdin := withCurrentDir(t, st.Payload, repo)

	runOnce(t, bin, stdin)

	durations := make([]time.Duration, 0, samples)
	for range samples {
		durations = append(durations, runOnce(t, bin, stdin))
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })

	p50 := durations[len(durations)*50/100]
	p99 := durations[len(durations)*99/100]
	t.Logf("%d renders, execve to exit: p50 %v, p99 %v, max %v (§8.1 budget %v)",
		samples, p50.Round(time.Microsecond), p99.Round(time.Microsecond),
		durations[len(durations)-1].Round(time.Microsecond), budget)

	if p99 >= budget {
		t.Errorf("p99 = %v, over §8.1's %v budget.\n"+
			"Every stage §8.1 names is under 0.3ms and process start is 2–4ms, so a p99 this high\n"+
			"means something new is on the path: a subprocess, a lock, a cache, or a network dial.",
			p99.Round(time.Microsecond), budget)
	}
}

func TestFileCountDoesNotMatter(t *testing.T) {
	if os.Getenv("CC_STATUSLINE_P99_BIG_REPO") == "" {
		t.Skip("set CC_STATUSLINE_P99_BIG_REPO=1 (or run `make p99`) to build a 50k-file repository")
	}
	bin := buildBinary(t)
	st, _ := refstate.ByName("danger-92")

	small := benchRepo(t)
	big := benchRepo(t)
	populate(t, big, 50_000)

	median := func(dir string) time.Duration {
		stdin := withCurrentDir(t, st.Payload, dir)
		runOnce(t, bin, stdin)
		d := make([]time.Duration, 0, 40)
		for range 40 {
			d = append(d, runOnce(t, bin, stdin))
		}
		sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
		return d[len(d)/2]
	}

	ms, mb := median(small), median(big)
	t.Logf("median: 3-file repo %v, 50,000-file repo %v", ms.Round(time.Microsecond), mb.Round(time.Microsecond))

	if mb > 2*ms+time.Millisecond {
		t.Errorf("50,000 files cost %v against %v for three.\n"+
			"Something is enumerating the repository. §5.8 walks upward and reads .git/HEAD.", mb, ms)
	}
}

func runOnce(t *testing.T, bin string, stdin []byte) time.Duration {
	t.Helper()
	cmd := exec.Command(bin, "render")
	cmd.Stdin = bytes.NewReader(stdin)
	cmd.Stdout = new(bytes.Buffer)
	cmd.Stderr = new(bytes.Buffer)
	cmd.Env = []string{"COLUMNS=120"}

	start := time.Now()
	err := cmd.Run()
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("render: %v\nstderr: %s", err, cmd.Stderr.(*bytes.Buffer).String())
	}
	if cmd.Stdout.(*bytes.Buffer).Len() == 0 {
		t.Fatal("render produced no output; the measurement is of nothing")
	}
	return elapsed
}

func buildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "cc-statusline")
	cmd := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w", "-o", bin, "..")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

func benchRepo(t *testing.T) string {
	t.Helper()
	root := syntheticRepo(t, "main")
	sub := filepath.Join(root, "internal", "line")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	return sub
}

func populate(t *testing.T, dir string, n int) {
	t.Helper()
	for i := range n {
		d := filepath.Join(dir, "pkg"+strconv.Itoa(i/200))
		if i%200 == 0 {
			if err := os.MkdirAll(d, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		f := filepath.Join(d, "f"+strconv.Itoa(i)+".go")
		if err := os.WriteFile(f, []byte("package p\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func withCurrentDir(t *testing.T, raw []byte, dir string) []byte {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	ws, ok := m["workspace"].(map[string]any)
	if !ok {
		ws = map[string]any{}
		m["workspace"] = ws
	}
	ws["current_dir"] = dir
	ws["project_dir"] = dir
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
