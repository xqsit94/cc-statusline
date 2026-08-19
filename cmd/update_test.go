package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/xqsit94/cc-statusline/internal/updater"
)

func withUpdaterFakes(t *testing.T, latest func(string) (updater.Release, error), fetch func(string, string, string, string) ([]byte, error), install func(string, []byte) error) {
	t.Helper()
	origLatest, origFetch, origInstall := updaterLatest, updaterFetch, updaterInstall
	updaterLatest, updaterFetch, updaterInstall = latest, fetch, install
	t.Cleanup(func() {
		updaterLatest, updaterFetch, updaterInstall = origLatest, origFetch, origInstall
	})
}

func withVersion(t *testing.T, v string) {
	t.Helper()
	orig := version
	version = v
	t.Cleanup(func() { version = orig })
}

func runUpdate(args ...string) (string, string, int) {
	var out, errb bytes.Buffer
	code := Update(args, &out, &errb)
	return out.String(), errb.String(), code
}

func neverFetch(t *testing.T) func(string, string, string, string) ([]byte, error) {
	return func(repo, tag, goos, goarch string) ([]byte, error) {
		t.Fatal("Fetch should not be called")
		return nil, nil
	}
}

func neverInstall(t *testing.T) func(string, []byte) error {
	return func(target string, binary []byte) error {
		t.Fatal("Install should not be called")
		return nil
	}
}

func TestUpdateReportsAlreadyLatest(t *testing.T) {
	withVersion(t, "v0.2.0")
	withUpdaterFakes(t,
		func(repo string) (updater.Release, error) { return updater.Release{TagName: "v0.2.0"}, nil },
		neverFetch(t), neverInstall(t))

	out, errOut, code := runUpdate()
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	if !strings.Contains(out, "already running the latest version") {
		t.Errorf("output = %q, want it to say already latest", out)
	}
}

func TestUpdateCheckOnlyDoesNotInstall(t *testing.T) {
	withVersion(t, "v0.1.0")
	withUpdaterFakes(t,
		func(repo string) (updater.Release, error) { return updater.Release{TagName: "v0.2.0"}, nil },
		neverFetch(t), neverInstall(t))

	out, errOut, code := runUpdate("--check")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	if !strings.Contains(out, "v0.2.0") || !strings.Contains(out, "--force") {
		t.Errorf("output = %q, want it to name v0.2.0 and --force", out)
	}
}

func TestUpdateWithoutForceDoesNotInstall(t *testing.T) {
	withVersion(t, "v0.1.0")
	withUpdaterFakes(t,
		func(repo string) (updater.Release, error) { return updater.Release{TagName: "v0.2.0"}, nil },
		neverFetch(t), neverInstall(t))

	out, errOut, code := runUpdate()
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	if !strings.Contains(out, "--force") {
		t.Errorf("output = %q, want it to point at --force", out)
	}
}

func TestUpdateForceInstallsAndReportsIt(t *testing.T) {
	withVersion(t, "v0.1.0")
	var installedTarget string
	var installedBinary []byte
	withUpdaterFakes(t,
		func(repo string) (updater.Release, error) { return updater.Release{TagName: "v0.2.0"}, nil },
		func(repo, tag, goos, goarch string) ([]byte, error) { return []byte("new-binary"), nil },
		func(target string, binary []byte) error {
			installedTarget, installedBinary = target, binary
			return nil
		})

	out, errOut, code := runUpdate("--force")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	if string(installedBinary) != "new-binary" {
		t.Errorf("Install got binary %q, want %q", installedBinary, "new-binary")
	}
	if installedTarget == "" {
		t.Error("Install was not called with a target path")
	}
	if !strings.Contains(out, "v0.2.0") {
		t.Errorf("output = %q, want it to name v0.2.0", out)
	}
}

func TestUpdateLatestErrorIsReportedAndExitsNonZero(t *testing.T) {
	withVersion(t, "v0.1.0")
	withUpdaterFakes(t,
		func(repo string) (updater.Release, error) { return updater.Release{}, errors.New("network down") },
		neverFetch(t), neverInstall(t))

	_, errOut, code := runUpdate()
	if code == 0 {
		t.Fatal("exit 0, want non-zero")
	}
	if !strings.Contains(errOut, "network down") {
		t.Errorf("stderr = %q, want it to mention the underlying error", errOut)
	}
}

func TestUpdateFetchErrorIsReportedAndExitsNonZero(t *testing.T) {
	withVersion(t, "v0.1.0")
	withUpdaterFakes(t,
		func(repo string) (updater.Release, error) { return updater.Release{TagName: "v0.2.0"}, nil },
		func(repo, tag, goos, goarch string) ([]byte, error) { return nil, errors.New("checksum mismatch") },
		neverInstall(t))

	_, errOut, code := runUpdate("--force")
	if code == 0 {
		t.Fatal("exit 0, want non-zero")
	}
	if !strings.Contains(errOut, "checksum mismatch") {
		t.Errorf("stderr = %q, want it to mention the underlying error", errOut)
	}
}

func TestUpdateDevBuildSkipsComparisonButStillGatesOnForce(t *testing.T) {
	withVersion(t, "dev")
	withUpdaterFakes(t,
		func(repo string) (updater.Release, error) { return updater.Release{TagName: "v0.2.0"}, nil },
		neverFetch(t), neverInstall(t))

	out, errOut, code := runUpdate()
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	if strings.Contains(out, "already running the latest version") {
		t.Errorf("output = %q, a dev build should never claim to be up to date", out)
	}
}
