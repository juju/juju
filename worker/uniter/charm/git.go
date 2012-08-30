package charm

import (
	"fmt"
	"io"
	"io/ioutil"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/trivial"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GitDir exposes a specialized subset of git operations on a directory.
type GitDir struct {
	path string
}

// NewGitDir creates a new GitDir at path. It does not touch the filesystem.
func NewGitDir(path string) *GitDir {
	return &GitDir{path}
}

// Path returns the directory path.
func (d *GitDir) Path() string {
	return d.path
}

// Exists returns true if the directory exists.
func (d *GitDir) Exists() (bool, error) {
	if _, err := os.Stat(d.path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Init ensures that a git repository exists in the directory.
func (d *GitDir) Init() error {
	if err := trivial.EnsureDir(d.path); err != nil {
		return err
	}
	return d.cmd("init")
}

// AddAll ensures that the next commit will reflect the current contents of
// the directory. Empty directories will be preserved by inserting and tracking
// empty files named .empty.
func (d *GitDir) AddAll() error {
	walker := func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !fi.IsDir() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		if _, err := f.Readdir(1); err != nil {
			if err == io.EOF {
				empty := filepath.Join(path, ".empty")
				return ioutil.WriteFile(empty, nil, 0644)
			}
			return err
		}
		return nil
	}
	if err := filepath.Walk(d.path, walker); err != nil {
		return err
	}
	return d.cmd("add", "-A", ".")
}

// Commitf commits a new revision to the repository with the supplied message.
func (d *GitDir) Commitf(format string, args ...interface{}) error {
	return d.cmd("commit", "--allow-empty", "-m", fmt.Sprintf(format, args...))
}

// Clone creates a new GitDir at the specified path, with history cloned
// from the existing GitDir. It does not check out any files.
func (d *GitDir) Clone(path string) (*GitDir, error) {
	if err := d.cmd("clone", "--no-checkout", ".", path); err != nil {
		return nil, err
	}
	return &GitDir{path}, nil
}

// Pull pulls from the supplied GitDir.
func (d *GitDir) Pull(src *GitDir) error {
	return d.cmd("pull", src.path)
}

// Recover deletes the lock file, and soft-resets the directory, allowing
// the client to resume operations that were unexpectedly aborted. If no
// lock file is present, it does nothing.
func (d *GitDir) Recover() error {
	if err := os.Remove(filepath.Join(d.path, ".git", "index.lock")); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return d.cmd("reset", "--soft")
}

// Revert removes unversioned files and reverts everything else to its state
// as of the most recent commit.
func (d *GitDir) Revert() error {
	if err := d.cmd("reset", "--hard", "ORIG_HEAD"); err != nil {
		return err
	}
	return d.cmd("clean", "-f", "-f", "-d")
}

// Log returns a highly compacted history of the directory.
func (d *GitDir) Log() ([]string, error) {
	cmd := exec.Command("git", "--no-pager", "log", "--oneline")
	cmd.Dir = d.path
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}
	trim := strings.TrimRight(string(out), "\n")
	return strings.Split(trim, "\n"), nil
}

// cmd runs the specified command inside the directory. Errors will be logged
// in detail.
func (d *GitDir) cmd(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = d.path
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("git command failed: %s\npath: %s\nargs: %#v\n%s", err, d.path, args, string(out))
		return fmt.Errorf("git %s failed: %s", args[0], err)
	}
	return nil
}
