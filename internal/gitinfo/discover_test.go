package gitinfo

import (
	"os"
	"path/filepath"
	"testing"
)

func mkGit(t *testing.T, root string, files map[string]string) string {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestDiscoverBranch(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"attached", map[string]string{".git/HEAD": "ref: refs/heads/main\n"}, "main"},
		{"slashes in the name", map[string]string{".git/HEAD": "ref: refs/heads/feat/auth\n"}, "feat/auth"},
		{"no trailing newline", map[string]string{".git/HEAD": "ref: refs/heads/main"}, "main"},
		{
			"detached HEAD shows a short sha",
			map[string]string{".git/HEAD": "1f79f05ab3c4d5e6f708192a3b4c5d6e7f809192\n"},
			"1f79f05",
		},
		{
			"rebase-merge wins over a detached HEAD",
			map[string]string{
				".git/HEAD":                   "1f79f05ab3c4d5e6f708192a3b4c5d6e7f809192\n",
				".git/rebase-merge/head-name": "refs/heads/feature\n",
			},
			"feature",
		},
		{
			"rebase-apply is handled too",
			map[string]string{
				".git/HEAD":                   "1f79f05ab3c4d5e6f708192a3b4c5d6e7f809192\n",
				".git/rebase-apply/head-name": "refs/heads/patched\n",
			},
			"patched",
		},
		{
			"unreadable HEAD yields nothing",
			map[string]string{".git/config": "[core]\n\tbare = true\n"},
			"",
		},
		{"garbage HEAD yields nothing", map[string]string{".git/HEAD": "not a ref\n"}, ""},
		{"a ref outside refs/heads shows its last component",
			map[string]string{".git/HEAD": "ref: refs/tags/v1.0\n"}, "v1.0"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := mkGit(t, t.TempDir(), tc.files)
			got := Discover(nil, root)
			if !got.Found {
				t.Fatal("Found = false, want true")
			}
			if got.Branch != tc.want {
				t.Errorf("Branch = %q, want %q", got.Branch, tc.want)
			}
		})
	}
}

func TestDiscoverWalksUpward(t *testing.T) {
	root := mkGit(t, t.TempDir(), map[string]string{".git/HEAD": "ref: refs/heads/main\n"})
	deep := filepath.Join(root, "a", "b", "c", "d")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := Discover(nil, deep); got.Branch != "main" {
		t.Errorf("Branch = %q from a nested directory, want %q", got.Branch, "main")
	}
}

func TestDiscoverGitdirFile(t *testing.T) {
	t.Run("absolute", func(t *testing.T) {
		dir := t.TempDir()
		real := filepath.Join(dir, "realgit")
		mkGit(t, dir, map[string]string{"realgit/HEAD": "ref: refs/heads/wt\n"})

		work := filepath.Join(dir, "work")
		os.MkdirAll(work, 0o755)
		os.WriteFile(filepath.Join(work, ".git"), []byte("gitdir: "+real+"\n"), 0o644)

		if got := Discover(nil, work); got.Branch != "wt" {
			t.Errorf("Branch = %q, want %q", got.Branch, "wt")
		}
	})

	t.Run("relative — the submodule form", func(t *testing.T) {
		dir := t.TempDir()
		mkGit(t, dir, map[string]string{"modules/sub/HEAD": "ref: refs/heads/subbranch\n"})

		work := filepath.Join(dir, "sub")
		os.MkdirAll(work, 0o755)
		os.WriteFile(filepath.Join(work, ".git"), []byte("gitdir: ../modules/sub\n"), 0o644)

		if got := Discover(nil, work); got.Branch != "subbranch" {
			t.Errorf("Branch = %q, want %q", got.Branch, "subbranch")
		}
	})

	t.Run("dangling pointer is not found", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: /nonexistent/elsewhere\n"), 0o644)
		if got := Discover(nil, dir); got.Found {
			t.Error("Found = true for a gitdir pointing nowhere")
		}
	})

	t.Run("a file that is not a gitdir pointer", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, ".git"), []byte("hello\n"), 0o644)
		if got := Discover(nil, dir); got.Found {
			t.Error("Found = true for a .git file with no gitdir: prefix")
		}
	})
}

func TestDiscoverEnvironment(t *testing.T) {
	root := mkGit(t, t.TempDir(), map[string]string{".git/HEAD": "ref: refs/heads/main\n"})

	t.Run("GIT_DIR skips the walk", func(t *testing.T) {
		other := mkGit(t, t.TempDir(), map[string]string{"custom/HEAD": "ref: refs/heads/explicit\n"})
		env := map[string]string{"GIT_DIR": filepath.Join(other, "custom")}
		if got := Discover(env, root); got.Branch != "explicit" {
			t.Errorf("Branch = %q, want %q", got.Branch, "explicit")
		}
	})

	t.Run("a GIT_DIR that does not exist is not a reason to search elsewhere", func(t *testing.T) {
		env := map[string]string{"GIT_DIR": "/nonexistent/git"}
		if got := Discover(env, root); got.Found {
			t.Error("Found = true; git itself would fail here rather than fall back")
		}
	})

	t.Run("GIT_CEILING_DIRECTORIES stops the walk", func(t *testing.T) {
		deep := filepath.Join(root, "a", "b")
		os.MkdirAll(deep, 0o755)
		env := map[string]string{"GIT_CEILING_DIRECTORIES": filepath.Join(root, "a")}
		if got := Discover(env, deep); got.Found {
			t.Error("Found = true, want the walk to stop at the ceiling")
		}
	})
}

func TestDiscoverAbsences(t *testing.T) {
	t.Run("no repository anywhere above", func(t *testing.T) {
		if got := Discover(nil, t.TempDir()); got.Found {
			t.Errorf("Found = true at %q", got.GitDir)
		}
	})

	t.Run("empty start directory", func(t *testing.T) {
		if got := Discover(nil, ""); got.Found {
			t.Error("Found = true for an empty start directory")
		}
	})

	t.Run("a directory that no longer exists", func(t *testing.T) {
		if got := Discover(nil, "/nonexistent/deleted/mid-session"); got.Found {
			t.Error("Found = true for a deleted directory")
		}
	})
}
