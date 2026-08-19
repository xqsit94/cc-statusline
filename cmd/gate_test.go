package cmd

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	"github.com/xqsit94/cc-statusline/internal/refstate"
)

func TestGateASCIISetIsPure7Bit(t *testing.T) {
	for _, width := range []int{40, 80, 120, 200} {
		for _, st := range refstate.All() {
			var out bytes.Buffer
			code := Preview([]string{"--icons", "ascii", "--bare",
				"--width", strconv.Itoa(width), "--state", st.Name},
				map[string]string{"TERM": "xterm-256color", "COLORTERM": "truecolor"},
				strings.NewReader(""), &out)
			if code != 0 {
				t.Fatalf("%s at %d: exit %d\n%s", st.Name, width, code, out.String())
			}
			for i, b := range out.Bytes() {
				if b >= 0x80 {
					t.Errorf("%s at %d: byte 0x%02x at offset %d is not ASCII:\n%q",
						st.Name, width, b, i, out.String())
					break
				}
			}
		}
	}
}

func TestGateNoColorRendersLikePlain(t *testing.T) {
	colourTerm := map[string]string{"TERM": "xterm-256color", "COLORTERM": "truecolor"}

	var plain, nocolor bytes.Buffer
	if code := Preview([]string{"--width", "120", "--plain", "--bare"},
		colourTerm, strings.NewReader(""), &plain); code != 0 {
		t.Fatalf("--plain: exit %d", code)
	}

	env := map[string]string{"NO_COLOR": "1"}
	for k, v := range colourTerm {
		env[k] = v
	}
	if code := Preview([]string{"--width", "120", "--bare"},
		env, strings.NewReader(""), &nocolor); code != 0 {
		t.Fatalf("NO_COLOR=1: exit %d", code)
	}

	if plain.String() != nocolor.String() {
		t.Errorf("NO_COLOR=1 and --plain differ:\n--plain:\n%s\nNO_COLOR:\n%s",
			plain.String(), nocolor.String())
	}
	for name, out := range map[string]string{"--plain": plain.String(), "NO_COLOR=1": nocolor.String()} {
		if strings.Contains(out, "\x1b[") {
			t.Errorf("%s emitted an SGR escape:\n%q", name, out)
		}
	}
	if !strings.Contains(plain.String(), "Claude") {
		t.Fatalf("--plain rendered no recognisable status line:\n%s", plain.String())
	}
}

func TestGateAmbiguousWidthFlagIsEffective(t *testing.T) {
	render := func(ambiguous string) string {
		var out bytes.Buffer
		code := Preview([]string{"--width", "120", "--plain", "--bare",
			"--icons", "unicode", "--ambiguous", ambiguous, "--state", "danger-92"},
			map[string]string{"TERM": "xterm-256color"}, strings.NewReader(""), &out)
		if code != 0 {
			t.Fatalf("--ambiguous %s: exit %d\n%s", ambiguous, code, out.String())
		}
		return out.String()
	}

	one, two := render("1"), render("2")
	if one == two {
		t.Fatalf("--ambiguous 1 and 2 render identically; the flag is doing nothing:\n%s", one)
	}
	if !strings.Contains(two, "▒") || strings.Contains(one, "▒") {
		t.Errorf("§5.6's empty-cell swap did not happen:\n amb1: %s\n amb2: %s", one, two)
	}
}
