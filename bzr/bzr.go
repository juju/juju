// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package bzr offers an interface to manage branches of the Bazaar VCS.
package bzr

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
)

// Branch represents a Bazaar branch.
type Branch struct {
	location string
	env      []string
}

// New returns a new Branch for the Bazaar branch at location.
func New(location string) *Branch {
	b := &Branch{location, cenv()}
	if _, err := os.Stat(location); err == nil {
		stdout, _, err := b.bzr("root")
		if err == nil {
			b.location = strings.TrimRight(string(stdout), "\n")
		}
	}
	return b
}

// cenv returns a copy of the current process environment with LC_ALL=C.
func cenv() []string {
	env := os.Environ()
	for i, pair := range env {
		if strings.HasPrefix(pair, "LC_ALL=") {
			env[i] = "LC_ALL=C"
			return env
		}
	}
	return append(env, "LC_ALL=C")
}

// Location returns the location of branch b.
func (b *Branch) Location() string {
	return b.location
}

// Join returns b's location with parts appended as path components.
// In other words, if b's location is "lp:foo", and parts is {"bar, baz"},
// Join returns "lp:foo/bar/baz".
func (b *Branch) Join(parts ...string) string {
	return path.Join(append([]string{b.location}, parts...)...)
}

func (b *Branch) bzr(subcommand string, args ...string) (stdout, stderr []byte, err error) {
	cmd := exec.Command("bzr", append([]string{subcommand}, args...)...)
	if _, err := os.Stat(b.location); err == nil {
		cmd.Dir = b.location
	}
	errbuf := &bytes.Buffer{}
	cmd.Stderr = errbuf
	cmd.Env = b.env
	stdout, err = cmd.Output()
	// Some commands fail with exit status 0 (e.g. bzr root). :-(
	if err != nil || bytes.Contains(errbuf.Bytes(), []byte("ERROR")) {
		var errmsg string
		if err != nil {
			errmsg = err.Error()
		}
		return nil, nil, fmt.Errorf(`error running "bzr %s": %s%s%s`, subcommand, stdout, errbuf.Bytes(), errmsg)
	}
	return stdout, errbuf.Bytes(), err
}

// Init intializes a new branch at b's location.
func (b *Branch) Init() error {
	_, _, err := b.bzr("init", b.location)
	return err
}

// Add adds to b the path resultant from calling b.Join(parts...).
func (b *Branch) Add(parts ...string) error {
	_, _, err := b.bzr("add", b.Join(parts...))
	return err
}

// Commit commits pending changes into b.
func (b *Branch) Commit(message string) error {
	_, _, err := b.bzr("commit", "-q", "-m", message)
	return err
}

// RevisionId returns the Bazaar revision id for the tip of b.
func (b *Branch) RevisionId() (string, error) {
	stdout, stderr, err := b.bzr("revision-info", "-d", b.location)
	if err != nil {
		return "", err
	}
	pair := bytes.Fields(stdout)
	if len(pair) != 2 {
		return "", fmt.Errorf(`invalid output from "bzr revision-info": %s%s`, stdout, stderr)
	}
	id := string(pair[1])
	if id == "null:" {
		return "", fmt.Errorf("branch has no content")
	}
	return id, nil
}

// PushLocation returns the default push location for b.
func (b *Branch) PushLocation() (string, error) {
	stdout, _, err := b.bzr("info", b.location)
	if err != nil {
		return "", err
	}
	if i := bytes.Index(stdout, []byte("push branch:")); i >= 0 {
		return string(stdout[i+13 : i+bytes.IndexAny(stdout[i:], "\r\n")]), nil
	}
	return "", fmt.Errorf("no push branch location defined")
}

// PushAttr holds options for the Branch.Push method.
type PushAttr struct {
	Location string // Location to push to. Use the default push location if empty.
	Remember bool   // Whether to remember the location being pushed to as the default.
}

// Push pushes any new revisions in b to attr.Location if that's
// provided, or to the default push location otherwise.
// See PushAttr for other options.
func (b *Branch) Push(attr *PushAttr) error {
	var args []string
	if attr != nil {
		if attr.Remember {
			args = append(args, "--remember")
		}
		if attr.Location != "" {
			args = append(args, attr.Location)
		}
	}
	_, _, err := b.bzr("push", args...)
	return err
}

// CheckClean returns an error if 'bzr status' is not clean.
func (b *Branch) CheckClean() error {
	stdout, _, err := b.bzr("status", b.location)
	if err != nil {
		return err
	}
	if bytes.Count(stdout, []byte{'\n'}) == 1 && bytes.Contains(stdout, []byte(`See "bzr shelve --list" for details.`)) {
		return nil // Shelves are fine.
	}
	if len(stdout) > 0 {
		return fmt.Errorf("branch is not clean (bzr status)")
	}
	return nil
}
