package testing

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/charm"
	"os"
	"os/exec"
	"path/filepath"
)

var srcRepo = os.ExpandEnv("$GOPATH/src/launchpad.net/juju/go/testing/repo")

type Repo struct {
	path string
}

func clone(c *C, src string) string {
	base := c.MkDir()
	err := exec.Command("cp", "-r", src, base).Run()
	c.Assert(err, IsNil)
	return filepath.Join(base, filepath.Base(src))
}

func NewRepo(c *C) *Repo {
	return &Repo{clone(c, srcRepo)}
}

func (r *Repo) DirPath(name string) string {
	return filepath.Join(r.path, "series", name)
}

func (r *Repo) Dir(c *C, name string) *charm.Dir {
	ch, err := charm.ReadDir(r.DirPath(name))
	c.Assert(err, IsNil)
	return ch
}

func (r *Repo) ClonedDirPath(c *C, name string) string {
	return clone(c, r.DirPath(name))
}

func (r *Repo) ClonedDir(c *C, name string) *charm.Dir {
	ch, err := charm.ReadDir(r.ClonedDirPath(c, name))
	c.Assert(err, IsNil)
	return ch
}

func (r *Repo) BundlePath(c *C, name string) string {
	dir := r.Dir(c, name)
	path := filepath.Join(c.MkDir(), "bundle.charm")
	file, err := os.Create(path)
	c.Assert(err, IsNil)
	defer file.Close()
	err = dir.BundleTo(file)
	c.Assert(err, IsNil)
	return path
}

func (r *Repo) Bundle(c *C, name string) *charm.Bundle {
	ch, err := charm.ReadBundle(r.BundlePath(c, name))
	c.Assert(err, IsNil)
	return ch
}
