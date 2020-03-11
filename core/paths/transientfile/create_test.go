// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//+build !windows

package transientfile

import (
	"os"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type transientFileSuite struct{}

var _ = gc.Suite(&transientFileSuite{})

func (s *transientFileSuite) TestCreateTransientFile(c *gc.C) {
	transientDir := c.MkDir()
	f, err := Create(transientDir, "foo.test")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(f.Close(), jc.ErrorIsNil)

	expPath := filepath.Join(transientDir, "foo.test")
	s.assertFileExists(c, expPath)
}

func (s *transientFileSuite) TestCreateTransientFileInSubdirectory(c *gc.C) {
	transientDir := c.MkDir()
	f, err := Create(transientDir, filepath.Join("1", "2", "foo.test"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(f.Close(), jc.ErrorIsNil)

	expPath := filepath.Join(transientDir, "1", "2", "foo.test")
	s.assertFileExists(c, expPath)
}

func (*transientFileSuite) assertFileExists(c *gc.C, path string) {
	stat, err := os.Stat(path)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("stat failed for %q", path))

	c.Assert(stat.IsDir(), jc.IsFalse, gc.Commentf("%q seems to be a directory", path))
}
