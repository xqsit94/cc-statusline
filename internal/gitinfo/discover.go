package gitinfo

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Info struct {
	Found  bool
	GitDir string
	Branch string
}

const maxWalk = 64

var sha40 = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

func Discover(env map[string]string, startDir string) Info {
	gitDir, ok := locate(env, startDir)
	if !ok {
		return Info{}
	}
	return Info{Found: true, GitDir: gitDir, Branch: readBranch(gitDir)}
}

func locate(env map[string]string, startDir string) (string, bool) {
	if dir := env["GIT_DIR"]; dir != "" {
		if isDir(dir) {
			return dir, true
		}
		return "", false
	}
	if startDir == "" {
		return "", false
	}

	ceilings := ceilingSet(env["GIT_CEILING_DIRECTORIES"])
	dir := filepath.Clean(startDir)

	for i := 0; i < maxWalk; i++ {
		if ceilings[dir] {
			return "", false
		}
		if resolved, ok := resolveDotGit(filepath.Join(dir, ".git")); ok {
			return resolved, true
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
	return "", false
}

func resolveDotGit(dotGit string) (string, bool) {
	st, err := os.Lstat(dotGit)
	if err != nil {
		return "", false
	}

	if st.IsDir() {
		return dotGit, true
	}
	if st.Mode()&os.ModeSymlink != 0 {
		if target, err := os.Stat(dotGit); err == nil && target.IsDir() {
			return dotGit, true
		}
		return "", false
	}

	b, err := os.ReadFile(dotGit)
	if err != nil {
		return "", false
	}
	target, ok := strings.CutPrefix(strings.TrimSpace(string(b)), "gitdir:")
	if !ok {
		return "", false
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return "", false
	}

	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(dotGit), target)
	}
	if !isDir(target) {
		return "", false
	}
	return filepath.Clean(target), true
}

func readBranch(gitDir string) string {
	if name, ok := readRebaseHead(gitDir); ok {
		return name
	}

	b, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return ""
	}
	head := strings.TrimSpace(string(b))

	if ref, ok := strings.CutPrefix(head, "ref:"); ok {
		ref = strings.TrimSpace(ref)
		if name, ok := strings.CutPrefix(ref, "refs/heads/"); ok {
			return name
		}
		return filepath.Base(ref)
	}

	if sha40.MatchString(head) {
		return head[:7]
	}

	return ""
}

func readRebaseHead(gitDir string) (string, bool) {
	for _, dir := range []string{"rebase-merge", "rebase-apply"} {
		b, err := os.ReadFile(filepath.Join(gitDir, dir, "head-name"))
		if err != nil {
			continue
		}
		name := strings.TrimSpace(string(b))
		if name, ok := strings.CutPrefix(name, "refs/heads/"); ok && name != "" {
			return name, true
		}
	}
	return "", false
}

func ceilingSet(v string) map[string]bool {
	if v == "" {
		return nil
	}
	out := map[string]bool{}
	for _, p := range filepath.SplitList(v) {
		if p != "" {
			out[filepath.Clean(p)] = true
		}
	}
	return out
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
