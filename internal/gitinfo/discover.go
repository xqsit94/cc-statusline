// Package gitinfo resolves the current branch name by reading git's files
// directly (PRD §5.8).
//
// No subprocess. Not as an optimisation — as the architectural constraint of
// §2.2: render never blocks and never forks. Resolving only the branch *name*
// is what makes that possible, because the name is in HEAD and nothing else
// needs reading. packed-refs is never touched.
package gitinfo

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Info is what one discovery found.
type Info struct {
	Found  bool   // a git directory was located
	GitDir string // the resolved git directory
	Branch string // branch name, short SHA when detached, or "" for a bare repo
}

// maxWalk caps the upward search. A path can be arbitrarily deep, and a
// symlink loop or a pathological mount can otherwise turn a status line into
// an unbounded stat() loop on every assistant turn.
const maxWalk = 64

// sha40 matches a detached HEAD's raw object id.
var sha40 = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

// Discover locates the git directory for startDir and reads the branch name.
//
// startDir comes from the payload's workspace.current_dir, never from
// os.Getwd(): a session whose directory was deleted underneath it makes Getwd
// return ENOENT, and the branch would vanish for a reason that has nothing to
// do with git.
//
// env is passed rather than read so the whole package stays testable without
// mutating the process environment, matching PRD §6.4's rule for capabilities.
//
// The branch name is returned in full. git.branch_max_len is applied by the
// branch segment, not here, because truncation needs the icon set's ellipsis
// glyph — `.` under ASCII, `…` otherwise — and this package deliberately knows
// nothing about how anything is displayed.
func Discover(env map[string]string, startDir string) Info {
	gitDir, ok := locate(env, startDir)
	if !ok {
		return Info{}
	}
	return Info{Found: true, GitDir: gitDir, Branch: readBranch(gitDir)}
}

// locate finds the git directory: GIT_DIR if set, otherwise an upward walk.
func locate(env map[string]string, startDir string) (string, bool) {
	if dir := env["GIT_DIR"]; dir != "" {
		if isDir(dir) {
			return dir, true
		}
		// An explicit GIT_DIR that does not exist is a user error, not an
		// invitation to search elsewhere — git itself would fail here too.
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
		if parent == dir { // filesystem root
			return "", false
		}
		dir = parent
	}
	return "", false
}

// resolveDotGit handles both forms of `.git`: a directory, or a file holding
// `gitdir: <path>` — which is what worktrees and submodules write.
func resolveDotGit(dotGit string) (string, bool) {
	st, err := os.Lstat(dotGit)
	if err != nil {
		return "", false
	}

	if st.IsDir() {
		return dotGit, true
	}
	if st.Mode()&os.ModeSymlink != 0 {
		// A symlinked .git resolves through Stat; if it points at a directory
		// it behaves like the directory form.
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

	// Submodules write a path relative to the containing directory. PRD §5.8
	// allows exactly one level of indirection — a chain of gitdir files is not
	// something git produces, and following it would reintroduce the unbounded
	// walk maxWalk exists to prevent.
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(dotGit), target)
	}
	if !isDir(target) {
		return "", false
	}
	return filepath.Clean(target), true
}

// readBranch reads the branch name from a resolved git directory.
func readBranch(gitDir string) string {
	// An interrupted rebase leaves HEAD detached at the commit being replayed,
	// which would render a meaningless SHA. head-name holds the branch the
	// user actually thinks they are on, so it wins.
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
		// A ref outside refs/heads/ is not a branch. Show its last component
		// rather than nothing — it is still where HEAD points.
		return filepath.Base(ref)
	}

	if sha40.MatchString(head) {
		return head[:7]
	}

	// A bare repository, or a HEAD we do not understand. Either way there is
	// no branch to show, and the segment renders empty rather than guessing.
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
