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

func doctorHome(t *testing.T) *home {
	t.Helper()
	h := newHome(t)
	cache := filepath.Join(h.dir, ".cache")
	h.env["XDG_CACHE_HOME"] = cache
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("HOME", h.dir)
	return h
}

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
	if !strings.Contains(out, "icons         ascii") {
		t.Errorf("icons not resolved to ascii:\n%s", out)
	}
	if !strings.Contains(out, "columns       120") {
		t.Errorf("columns not read from the environment:\n%s", out)
	}
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

func TestDoctorReadsTheLastError(t *testing.T) {
	h := doctorHome(t)
	writeConfigFile(t, h, "[general]\nicons = \"ascii\"\nmistyped_key = 1\n")

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

func TestRenderClearsTheLastErrorOnceFixed(t *testing.T) {
	h := doctorHome(t)
	writeConfigFile(t, h, "[general]\nmistyped_key = 1\n")

	var out bytes.Buffer
	Render(nil, h.env, strings.NewReader("{}"), &out)

	path := filepath.Join(h.dir, ".cache", "cc-statusline", lastErrorName)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the note was not recorded: %v", err)
	}

	writeConfigFile(t, h, "[general]\nicons = \"ascii\"\n")
	out.Reset()
	Render(nil, h.env, strings.NewReader("{}"), &out)

	if _, err := os.Stat(path); err == nil {
		body, _ := os.ReadFile(path)
		t.Errorf("a fixed problem is still reported:\n%s", body)
	}
}

func TestRenderStillHoldsTheFailureContractWhileRecording(t *testing.T) {
	h := doctorHome(t)
	writeConfigFile(t, h, "[general]\nmistyped_key = 1\n")

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
