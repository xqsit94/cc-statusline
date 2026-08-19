package updater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestCompare(t *testing.T) {
	cases := []struct {
		current, latest string
		want            int
	}{
		{"v0.1.0", "v0.1.0", 0},
		{"v0.1.0", "v0.2.0", -1},
		{"v0.2.0", "v0.1.0", 1},
		{"0.1.0", "v0.1.9", -1},
		{"v1.0.0", "v0.9.9", 1},
		{"v1.2", "v1.2.0", 0},
	}
	for _, c := range cases {
		if got := Compare(c.current, c.latest); got != c.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", c.current, c.latest, got, c.want)
		}
	}
}

func TestAssetNameMatchesGoreleaserTemplate(t *testing.T) {
	cases := []struct {
		goos, goarch, want string
	}{
		{"linux", "amd64", "cc-statusline_Linux_x86_64.tar.gz"},
		{"linux", "arm64", "cc-statusline_Linux_arm64.tar.gz"},
		{"darwin", "amd64", "cc-statusline_Darwin_x86_64.tar.gz"},
		{"darwin", "arm64", "cc-statusline_Darwin_arm64.tar.gz"},
	}
	for _, c := range cases {
		got, err := AssetName(c.goos, c.goarch)
		if err != nil {
			t.Fatalf("AssetName(%q, %q): %v", c.goos, c.goarch, err)
		}
		if got != c.want {
			t.Errorf("AssetName(%q, %q) = %q, want %q", c.goos, c.goarch, got, c.want)
		}
	}
}

func TestAssetNameRejectsUnsupportedPlatform(t *testing.T) {
	if _, err := AssetName("windows", "amd64"); err == nil {
		t.Error("AssetName(windows, amd64) = nil error, want one")
	}
}

func makeArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		hdr := &tar.Header{Name: name, Mode: 0o755, Size: int64(len(content))}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("writing tar header: %v", err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("writing tar body: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("closing tar writer: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("closing gzip writer: %v", err)
	}
	return buf.Bytes()
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestFetchVerifiesChecksumAndExtractsBinary(t *testing.T) {
	archive := makeArchive(t, map[string]string{"cc-statusline": "binary-bytes"})
	checksums := []byte(fmt.Sprintf("%s  cc-statusline_Linux_x86_64.tar.gz\n", sha256Hex(archive)))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch filepath.Base(r.URL.Path) {
		case "cc-statusline_Linux_x86_64.tar.gz":
			w.Write(archive)
		case "checksums.txt":
			w.Write(checksums)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	old := releaseBase
	releaseBase = srv.URL
	defer func() { releaseBase = old }()

	got, err := Fetch("owner/repo", "v1.0.0", "linux", "amd64")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if string(got) != "binary-bytes" {
		t.Errorf("Fetch returned %q, want %q", got, "binary-bytes")
	}
}

func TestFetchRejectsTamperedArchive(t *testing.T) {
	archive := makeArchive(t, map[string]string{"cc-statusline": "binary-bytes"})
	wrongChecksums := []byte(fmt.Sprintf("%s  cc-statusline_Linux_x86_64.tar.gz\n", sha256Hex([]byte("something-else"))))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch filepath.Base(r.URL.Path) {
		case "cc-statusline_Linux_x86_64.tar.gz":
			w.Write(archive)
		case "checksums.txt":
			w.Write(wrongChecksums)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	old := releaseBase
	releaseBase = srv.URL
	defer func() { releaseBase = old }()

	if _, err := Fetch("owner/repo", "v1.0.0", "linux", "amd64"); err == nil {
		t.Error("Fetch with a mismatched checksum returned nil error, want one")
	}
}

func TestFetchRejectsUnlistedAsset(t *testing.T) {
	archive := makeArchive(t, map[string]string{"cc-statusline": "binary-bytes"})
	checksums := []byte("deadbeef  some-other-file.tar.gz\n")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch filepath.Base(r.URL.Path) {
		case "cc-statusline_Linux_x86_64.tar.gz":
			w.Write(archive)
		case "checksums.txt":
			w.Write(checksums)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	old := releaseBase
	releaseBase = srv.URL
	defer func() { releaseBase = old }()

	if _, err := Fetch("owner/repo", "v1.0.0", "linux", "amd64"); err == nil {
		t.Error("Fetch against checksums.txt missing the asset returned nil error, want one")
	}
}

func TestLatest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"tag_name":"v0.2.0","html_url":"https://example.com/v0.2.0","body":"notes"}`))
	}))
	defer srv.Close()

	old := apiBase
	apiBase = srv.URL
	defer func() { apiBase = old }()

	rel, err := Latest("owner/repo")
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if rel.TagName != "v0.2.0" {
		t.Errorf("TagName = %q, want v0.2.0", rel.TagName)
	}
}

func TestInstallReplacesFileAtomicallyAndPreservesMode(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "cc-statusline")
	if err := os.WriteFile(target, []byte("old-binary"), 0o744); err != nil {
		t.Fatalf("seeding target: %v", err)
	}

	if err := Install(target, []byte("new-binary")); err != nil {
		t.Fatalf("Install: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("reading installed file: %v", err)
	}
	if string(got) != "new-binary" {
		t.Errorf("installed content = %q, want %q", got, "new-binary")
	}

	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o744 {
		t.Errorf("mode = %o, want %o", info.Mode().Perm(), 0o744)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading dir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("directory has %d entries after Install, want 1 (no leftover temp file)", len(entries))
	}
}

func TestInstallCreatesFileWithDefaultModeWhenTargetIsNew(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "cc-statusline")

	if err := Install(target, []byte("new-binary")); err != nil {
		t.Fatalf("Install: %v", err)
	}

	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("mode = %o, want %o", info.Mode().Perm(), 0o755)
	}
}
