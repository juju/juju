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
}

// New returns a new Branch for the Bazaar branch at location.
func New(location string) (*Branch, error) {
	b := &Branch{location}
	if _, err := os.Stat(location); err == nil {
		stdout, _, err := b.bzr("root")
		if err == nil {
			b.location = strings.TrimRight(string(stdout), "\n")
		} else if !strings.Contains(err.Error(), "Not a branch") {
			return nil, err
		}
	}
	return b, nil
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

func (b *Branch) bzr(args ...string) (stdout, stderr []byte, err error) {
	if len(args) == 0 {
		panic("no point in runing bzr without arguments here")
	}
	cmd := exec.Command("bzr", args...)
	if _, err := os.Stat(b.location); err == nil {
		cmd.Dir = b.location
	}
	errbuf := &bytes.Buffer{}
	cmd.Stderr = errbuf
	stdout, err = cmd.Output()
	// Some commands fail with exit status 0 (e.g. bzr root). :-(
	if err != nil || bytes.Contains(errbuf.Bytes(), []byte("ERROR")) {
		var errmsg string
		if err != nil {
			errmsg = err.Error()
		}
		return nil, nil, fmt.Errorf(`error running "bzr %s": %s%s%s`, args[0], stdout, errbuf.Bytes(), errmsg)
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
	args := []string{"push"}
	if attr != nil {
		if attr.Remember {
			args = append(args, "--remember")
		}
		if attr.Location != "" {
			args = append(args, attr.Location)
		}
	}
	_, _, err := b.bzr(args...)
	return err
}
