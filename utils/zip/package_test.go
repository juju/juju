// Copyright 2011-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package zip_test

import (
	"archive/zip"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing/testbase"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type BaseSuite struct {
	testbase.LoggingSuite
}

func (s *BaseSuite) makeZip(c *gc.C, creators ...creator) *zip.Reader {
	basePath := c.MkDir()
	for _, creator := range creators {
		creator.create(c, basePath)
	}
	defer os.RemoveAll(basePath)

	outPath := join(c.MkDir(), "test.zip")
	cmd := exec.Command("/bin/sh", "-c", fmt.Sprintf("cd %q; zip --fifo --symlinks -r %q .", basePath, outPath))
	output, err := cmd.CombinedOutput()
	c.Assert(err, gc.IsNil, gc.Commentf("Command output: %s", output))

	file, err := os.Open(outPath)
	c.Assert(err, gc.IsNil)
	s.AddCleanup(func(c *gc.C) {
		err := file.Close()
		c.Assert(err, gc.IsNil)
	})
	fileInfo, err := file.Stat()
	c.Assert(err, gc.IsNil)
	reader, err := zip.NewReader(file, fileInfo.Size())
	c.Assert(err, gc.IsNil)
	return reader
}

type creator interface {
	create(c *gc.C, basePath string)
	check(c *gc.C, basePath string)
}

func join(basePath, path string) string {
	return filepath.Join(basePath, filepath.FromSlash(path))
}

type dir struct {
	path string
	perm os.FileMode
}

func (d dir) create(c *gc.C, basePath string) {
	err := os.MkdirAll(join(basePath, d.path), d.perm)
	c.Assert(err, gc.IsNil)
}

func (d dir) check(c *gc.C, basePath string) {
	fileInfo, err := os.Lstat(join(basePath, d.path))
	c.Check(err, gc.IsNil)
	c.Check(fileInfo.Mode()&os.ModePerm, gc.Equals, d.perm)
}

type file struct {
	path string
	data string
	perm os.FileMode
}

func (f file) create(c *gc.C, basePath string) {
	err := ioutil.WriteFile(join(basePath, f.path), []byte(f.data), f.perm)
	c.Assert(err, gc.IsNil)
}

func (f file) check(c *gc.C, basePath string) {
	path := join(basePath, f.path)
	fileInfo, err := os.Lstat(path)
	if !c.Check(err, gc.IsNil) {
		return
	}
	mode := fileInfo.Mode()
	c.Check(mode&os.ModeType, gc.Equals, os.FileMode(0))
	c.Check(mode&os.ModePerm, gc.Equals, f.perm)
	data, err := ioutil.ReadFile(path)
	c.Check(err, gc.IsNil)
	c.Check(string(data), gc.Equals, f.data)
}

type symlink struct {
	path string
	data string
}

func (s symlink) create(c *gc.C, basePath string) {
	err := os.Symlink(s.data, join(basePath, s.path))
	c.Assert(err, gc.IsNil)
}

func (s symlink) check(c *gc.C, basePath string) {
	data, err := os.Readlink(join(basePath, s.path))
	c.Check(err, gc.IsNil)
	c.Check(data, gc.Equals, s.data)
}
