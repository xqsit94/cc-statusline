package updater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const BinaryName = "cc-statusline"

var (
	apiBase     = "https://api.github.com"
	releaseBase = "https://github.com"
	httpClient  = &http.Client{Timeout: 30 * time.Second}
)

type Release struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Body    string `json:"body"`
}

func Latest(repo string) (Release, error) {
	req, err := http.NewRequest(http.MethodGet, apiBase+"/repos/"+repo+"/releases/latest", nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("checking %s's latest release: %w", repo, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("checking %s's latest release: GitHub returned %s", repo, resp.Status)
	}

	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return Release{}, fmt.Errorf("decoding %s's latest release: %w", repo, err)
	}
	return rel, nil
}

// Compare returns -1, 0, or 1 as current is less than, equal to, or greater
// than latest. Both are compared as dot-separated integers, ignoring a
// leading "v" — good enough for this project's vMAJOR.MINOR.PATCH tags, and
// non-numeric components compare as 0 rather than failing.
func Compare(current, latest string) int {
	c := strings.Split(strings.TrimPrefix(current, "v"), ".")
	l := strings.Split(strings.TrimPrefix(latest, "v"), ".")
	n := len(c)
	if len(l) > n {
		n = len(l)
	}
	for i := 0; i < n; i++ {
		var cv, lv int
		if i < len(c) {
			cv, _ = strconv.Atoi(c[i])
		}
		if i < len(l) {
			lv, _ = strconv.Atoi(l[i])
		}
		if cv != lv {
			if cv < lv {
				return -1
			}
			return 1
		}
	}
	return 0
}

// AssetName mirrors .goreleaser.yaml's archive name_template exactly — the
// two must agree or Fetch asks the release for a filename it never publishes.
func AssetName(goos, goarch string) (string, error) {
	var os string
	switch goos {
	case "linux":
		os = "Linux"
	case "darwin":
		os = "Darwin"
	default:
		return "", fmt.Errorf("no release is published for %s", goos)
	}
	var arch string
	switch goarch {
	case "amd64":
		arch = "x86_64"
	case "arm64":
		arch = "arm64"
	default:
		return "", fmt.Errorf("no release is published for %s/%s", goos, goarch)
	}
	return fmt.Sprintf("%s_%s_%s.tar.gz", BinaryName, os, arch), nil
}

// Fetch downloads tag's release archive and checksums.txt for goos/goarch,
// verifies the archive's SHA-256 against checksums.txt, and returns the
// cc-statusline binary extracted from inside it — the same verification
// install.sh does before it ever unpacks a download.
func Fetch(repo, tag, goos, goarch string) ([]byte, error) {
	asset, err := AssetName(goos, goarch)
	if err != nil {
		return nil, err
	}
	base := fmt.Sprintf("%s/%s/releases/download/%s", releaseBase, repo, tag)

	archive, err := download(base + "/" + asset)
	if err != nil {
		return nil, err
	}
	checksums, err := download(base + "/checksums.txt")
	if err != nil {
		return nil, err
	}
	if err := verifyChecksum(archive, checksums, asset); err != nil {
		return nil, err
	}
	return extractBinary(archive, BinaryName)
}

func download(url string) ([]byte, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("downloading %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading %s: server returned %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func verifyChecksum(archive, checksums []byte, asset string) error {
	want := ""
	for _, line := range strings.Split(string(checksums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == asset {
			want = fields[0]
			break
		}
	}
	if want == "" {
		return fmt.Errorf("checksums.txt does not mention %s — refusing to install an unverified binary", asset)
	}

	sum := sha256.Sum256(archive)
	got := hex.EncodeToString(sum[:])
	if !strings.EqualFold(want, got) {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s — the download was corrupted or tampered with", asset, want, got)
	}
	return nil
}

func extractBinary(archive []byte, name string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("opening archive: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading archive: %w", err)
		}
		if hdr.Typeflag == tar.TypeReg && filepath.Base(hdr.Name) == name {
			return io.ReadAll(tr)
		}
	}
	return nil, fmt.Errorf("archive did not contain %s", name)
}

// Install atomically replaces target's contents with binary, preserving
// target's file mode. It writes to a temp file in target's own directory
// first and renames over target — one syscall, so a crash or a failed write
// leaves target exactly as it was, never half-written and never missing.
func Install(target string, binary []byte) error {
	mode := os.FileMode(0o755)
	if info, err := os.Stat(target); err == nil {
		mode = info.Mode()
	}

	dir := filepath.Dir(target)
	tmp, err := os.CreateTemp(dir, filepath.Base(target)+".new-*")
	if err != nil {
		return fmt.Errorf("creating a temp file next to %s: %w", target, err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(binary); err != nil {
		tmp.Close()
		return fmt.Errorf("writing %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("writing %s: %w", tmpPath, err)
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		return fmt.Errorf("setting %s's permissions: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, target); err != nil {
		return fmt.Errorf("installing over %s: %w", target, err)
	}
	return nil
}
