// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/utils"
)

var ErrConflict = errors.New("charm upgrade has conflicts")

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
	fi, err := os.Stat(d.path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if fi.IsDir() {
		return true, nil
	}
	return false, fmt.Errorf("%q is not a directory", d.path)
}

// Init ensures that a git repository exists in the directory.
func (d *GitDir) Init() error {
	if err := os.MkdirAll(d.path, 0755); err != nil {
		return err
	}
	commands := [][]string{
		{"init"},
		{"config", "user.email", "juju@localhost"},
		{"config", "user.name", "juju"},
	}
	for _, args := range commands {
		if err := d.cmd(args...); err != nil {
			return err
		}
	}
	return nil
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

	// special handling for addall, since there is an error condition that
	// we need to suppress
	return d.addAll()
}

// addAll runs "git add -A ."" and swallows errors about no matching files. This
// is to replicate the behavior of older versions of git that returned no error
// in that situation.
func (d *GitDir) addAll() error {
	args := []string{"add", "-A", "."}
	cmd := exec.Command("git", args...)
	cmd.Dir = d.path
	if out, err := cmd.CombinedOutput(); err != nil {
		output := string(out)
		// Swallow this specific error. It's a change in behavior from older
		// versions of git, and we want AddAll to be able to be used on empty
		// directories.
		if !strings.Contains(output, "pathspec '.' did not match any files") {
			return d.logError(err, string(out), args...)
		}
	}
	return nil
}

// Commitf commits a new revision to the repository with the supplied message.
func (d *GitDir) Commitf(format string, args ...interface{}) error {
	return d.cmd("commit", "--allow-empty", "-m", fmt.Sprintf(format, args...))
}

// Snapshotf adds all changes made since the last commit, including deletions
// and empty directories, and commits them using the supplied message.
func (d *GitDir) Snapshotf(format string, args ...interface{}) error {
	if err := d.AddAll(); err != nil {
		return err
	}
	return d.Commitf(format, args...)
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
	err := d.cmd("pull", src.path)
	if err != nil {
		if conflicted, e := d.Conflicted(); e == nil && conflicted {
			return ErrConflict
		}
	}
	return err
}

// Dirty returns true if the directory contains any uncommitted local changes.
func (d *GitDir) Dirty() (bool, error) {
	statuses, err := d.statuses()
	if err != nil {
		return false, err
	}
	return len(statuses) != 0, nil
}

// Conflicted returns true if the directory contains any conflicts.
func (d *GitDir) Conflicted() (bool, error) {
	statuses, err := d.statuses()
	if err != nil {
		return false, err
	}
	for _, st := range statuses {
		switch st {
		case "AA", "DD", "UU", "AU", "UA", "DU", "UD":
			return true, nil
		}
	}
	return false, nil
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
	out, err := cmd.Output()
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
		return d.logError(err, string(out), args...)
	}
	return nil
}

func (d *GitDir) logError(err error, output string, args ...string) error {
	log.Errorf("worker/uniter/charm: git command failed: %s\npath: %s\nargs: %#v\n%s",
		err, d.path, args, output)
	return fmt.Errorf("git %s failed: %s", args[0], err)
}

// statuses returns a list of XY-coded git statuses for the files in the directory.
func (d *GitDir) statuses() ([]string, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = d.path
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git status failed: %v", err)
	}
	statuses := []string{}
	for _, line := range strings.Split(string(out), "\n") {
		if line != "" {
			statuses = append(statuses, line[:2])
		}
	}
	return statuses, nil
}

// ReadCharmURL reads the charm identity file from the supplied GitDir.
func ReadCharmURL(d *GitDir) (*charm.URL, error) {
	path := filepath.Join(d.path, ".juju-charm")
	surl := ""
	if err := utils.ReadYaml(path, &surl); err != nil {
		return nil, err
	}
	return charm.ParseURL(surl)
}

// WriteCharmURL writes a charm identity file into the directory.
func WriteCharmURL(d *GitDir, url *charm.URL) error {
	return utils.WriteYaml(filepath.Join(d.path, ".juju-charm"), url.String())
}
