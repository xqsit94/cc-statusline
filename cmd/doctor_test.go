package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runDoctor(t *testing.T, h *home, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	code := Doctor(args, h.env, &out, &errb)
	return out.String(), errb.String(), code
}

// doctorHome adds a cache directory to the isolated environment. `doctor` reads
// the capture and the last-error file through os.Getenv-backed helpers shared
// with `capture`, so the process environment is redirected for the test.
func doctorHome(t *testing.T) *home {
	t.Helper()
	h := newHome(t)
	cache := filepath.Join(h.dir, ".cache")
	h.env["XDG_CACHE_HOME"] = cache
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("HOME", h.dir)
	return h
}

// TestDoctorOnACleanMachineExitsZero is §4.1: doctor exits 0 when things are
// merely absent. Nothing is installed, there is no config, there is no capture,
// and none of that is a failure — they are all states a user can be told about.
func TestDoctorOnACleanMachineExitsZero(t *testing.T) {
	h := doctorHome(t)

	out, errOut, code := runDoctor(t, h)
	if code != 0 {
		t.Fatalf("exit %d\nstderr: %s", code, errOut)
	}
	if errOut != "" {
		t.Errorf("stderr: %s", errOut)
	}
	for _, want := range []string{"install", "config", "capabilities", "payload"} {
		if !strings.Contains(out, want) {
			t.Errorf("no %q section:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "cc-statusline init") {
		t.Errorf("an uninstalled machine should be told what to run:\n%s", out)
	}
}

func TestDoctorSeesTheInstall(t *testing.T) {
	h := doctorHome(t)
	if _, errOut, code := runInit(t, h); code != 0 {
		t.Fatalf("init: exit %d: %s", code, errOut)
	}

	out, _, code := runDoctor(t, h)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out, fakeExe+" render") {
		t.Errorf("the installed command was not reported:\n%s", out)
	}
	if !strings.Contains(out, "is this build true") {
		t.Errorf("doctor should say whether the statusLine is us:\n%s", out)
	}
}

// TestDoctorReportsAForeignStatusLine is the diagnosis for "I installed it and
// nothing changed": something else owns the key.
func TestDoctorReportsAForeignStatusLine(t *testing.T) {
	h := doctorHome(t)
	h.seed(t, "existing-statusline.json")

	out, _, code := runDoctor(t, h)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out, "runs something else") {
		t.Errorf("expected the foreign command to be called out:\n%s", out)
	}
}

func TestDoctorReportsAnUneditableSettingsFile(t *testing.T) {
	h := doctorHome(t)
	h.seed(t, "line-comments.json")

	out, _, code := runDoctor(t, h)
	if code != 0 {
		t.Fatalf("a settings file with comments is not a doctor failure; exit %d", code)
	}
	if !strings.Contains(out, "not plain JSON") {
		t.Errorf("expected an explanation:\n%s", out)
	}
}

// TestDoctorExitsOneOnlyForAnUnparseableConfig is §4.1's exit contract: "0
// usable; 1 only if config is malformed". A typo'd key is repaired and stays
// exit 0; a file that is not TOML at all is the one case where what doctor
// reports and what render does could diverge.
func TestDoctorExitsOneOnlyForAnUnparseableConfig(t *testing.T) {
	t.Run("repaired keys stay usable", func(t *testing.T) {
		h := doctorHome(t)
		writeConfigFile(t, h, "[general]\nnot_a_real_key = 3\nicons = \"ascii\"\n")

		out, _, code := runDoctor(t, h)
		if code != 0 {
			t.Errorf("exit = %d, want 0 — an unknown key is repaired, not fatal", code)
		}
		if !strings.Contains(out, "not_a_real_key") {
			t.Errorf("the ignored key should be reported:\n%s", out)
		}
	})

	t.Run("unparseable is fatal", func(t *testing.T) {
		h := doctorHome(t)
		writeConfigFile(t, h, "this is not TOML at all [[[\n")

		out, _, code := runDoctor(t, h)
		if code != 1 {
			t.Errorf("exit = %d, want 1\n%s", code, out)
		}
	})
}

func TestDoctorReportsWhatDecidedTheCapabilities(t *testing.T) {
	h := doctorHome(t)
	h.env["CC_STATUSLINE_ASCII"] = "1"
	h.env["COLUMNS"] = "120"
	h.env["NO_COLOR"] = "1"

	out, _, code := runDoctor(t, h)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	// The resolved answers.
	if !strings.Contains(out, "icons         ascii") {
		t.Errorf("icons not resolved to ascii:\n%s", out)
	}
	if !strings.Contains(out, "columns       120") {
		t.Errorf("columns not read from the environment:\n%s", out)
	}
	// And why: §6.1's contract is four variables and a locale, and "why is it
	// ASCII" is only answerable if they are printed beside the answer.
	for _, want := range []string{"CC_STATUSLINE_ASCII", "NO_COLOR", "COLUMNS"} {
		if !strings.Contains(out, want) {
			t.Errorf("%s was not reported as the reason:\n%s", want, out)
		}
	}
}

func TestDoctorSaysWhenNothingWasSet(t *testing.T) {
	h := doctorHome(t)
	out, _, code := runDoctor(t, h)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out, "every value above is a default") {
		t.Errorf("an empty environment should say so rather than print a blank list:\n%s", out)
	}
}

// TestDoctorReadsTheLastError closes §7.1's loop: render records what it had to
// repair, doctor reports it.
func TestDoctorReadsTheLastError(t *testing.T) {
	h := doctorHome(t)
	writeConfigFile(t, h, "[general]\nicons = \"ascii\"\nmistyped_key = 1\n")

	// Render, which is what writes the file.
	var out bytes.Buffer
	if code := Render(nil, h.env, strings.NewReader("{}"), &out); code != 0 {
		t.Fatalf("render exit %d", code)
	}

	report, _, code := runDoctor(t, h)
	if code != 0 {
		t.Fatalf("doctor exit %d", code)
	}
	if !strings.Contains(report, "last error") {
		t.Errorf("no last-error section:\n%s", report)
	}
	if !strings.Contains(report, "mistyped_key") {
		t.Errorf("the recorded note was not read back:\n%s", report)
	}
}

// TestRenderClearsTheLastErrorOnceFixed is the other half, and the reason the
// file is rewritten rather than appended to: a corrected typo must stop being
// reported, or `doctor` accumulates a history of problems the user has already
// solved.
func TestRenderClearsTheLastErrorOnceFixed(t *testing.T) {
	h := doctorHome(t)
	writeConfigFile(t, h, "[general]\nmistyped_key = 1\n")

	var out bytes.Buffer
	Render(nil, h.env, strings.NewReader("{}"), &out)

	path := filepath.Join(h.dir, ".cache", "cc-statusline", lastErrorName)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the note was not recorded: %v", err)
	}

	// The user fixes it.
	writeConfigFile(t, h, "[general]\nicons = \"ascii\"\n")
	out.Reset()
	Render(nil, h.env, strings.NewReader("{}"), &out)

	if _, err := os.Stat(path); err == nil {
		body, _ := os.ReadFile(path)
		t.Errorf("a fixed problem is still reported:\n%s", body)
	}
}

// TestRenderStillHoldsTheFailureContractWhileRecording is the thing that would
// be easy to break by adding a filesystem write to the hot path. §3.3: exit 0,
// one non-empty line, nothing on stderr — even when the cache is unwritable.
func TestRenderStillHoldsTheFailureContractWhileRecording(t *testing.T) {
	h := doctorHome(t)
	writeConfigFile(t, h, "[general]\nmistyped_key = 1\n")

	// A file where the cache directory should be, so every write into it fails.
	cache := filepath.Join(h.dir, ".cache", "cc-statusline")
	if err := os.MkdirAll(filepath.Dir(cache), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cache, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if code := Render(nil, h.env, strings.NewReader("{}"), &out); code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if strings.TrimSpace(out.String()) == "" {
		t.Error("no line was rendered")
	}
}

func TestDoctorJSONIsMachineReadable(t *testing.T) {
	h := doctorHome(t)
	if _, _, code := runInit(t, h); code != 0 {
		t.Fatal("init failed")
	}

	out, errOut, code := runDoctor(t, h, "--json")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}

	var rep report
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !rep.Install.IsUs {
		t.Error("install.is_us should be true after init")
	}
	if rep.Capabilities.Columns == 0 {
		t.Error("capabilities.columns should always resolve to something")
	}
	if rep.Capabilities.Available <= 0 {
		t.Error("capabilities.available should be the real budget")
	}
}

func TestDoctorRejectsBadFlags(t *testing.T) {
	h := doctorHome(t)
	var out, errb bytes.Buffer
	if code := Doctor([]string{"--nonsense"}, h.env, &out, &errb); code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
}

// TestDoctorReportsPayloadDrift is §3.1.2's drift detection made visible: a
// captured payload with a key this build does not model is how a contract
// change first shows up in the field.
func TestDoctorReportsPayloadDrift(t *testing.T) {
	h := doctorHome(t)
	capturePath := filepath.Join(h.dir, ".cache", "cc-statusline", "last-payload.json")
	if err := os.MkdirAll(filepath.Dir(capturePath), 0o755); err != nil {
		t.Fatal(err)
	}
	wrapped := `{"captured_at":"2026-08-05T00:00:00Z","env":{},"payload":{"model":{"display_name":"Opus"},"brand_new_field":42}}`
	if err := os.WriteFile(capturePath, []byte(wrapped), 0o600); err != nil {
		t.Fatal(err)
	}

	out, _, code := runDoctor(t, h)
	if code != 0 {
		t.Fatalf("payload drift is not a failure; exit %d", code)
	}
	if !strings.Contains(out, "brand_new_field") {
		t.Errorf("the unknown key was not reported:\n%s", out)
	}
	if !strings.Contains(out, "> ") {
		t.Errorf("doctor should render the captured payload:\n%s", out)
	}
}

func writeConfigFile(t *testing.T, h *home, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(h.config), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(h.config, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
