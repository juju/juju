// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build !windows

package transientfile

import (
	"os"
	"path/filepath"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
)

type transientFileSuite struct{}

var _ = tc.Suite(&transientFileSuite{})

func (s *transientFileSuite) TestCreateTransientFile(c *tc.C) {
	transientDir := c.MkDir()
	f, err := Create(transientDir, "foo.test")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(f.Close(), jc.ErrorIsNil)

	expPath := filepath.Join(transientDir, "foo.test")
	s.assertFileExists(c, expPath)
}

func (s *transientFileSuite) TestCreateTransientFileInSubdirectory(c *tc.C) {
	transientDir := c.MkDir()
	f, err := Create(transientDir, filepath.Join("1", "2", "foo.test"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(f.Close(), jc.ErrorIsNil)

	expPath := filepath.Join(transientDir, "1", "2", "foo.test")
	s.assertFileExists(c, expPath)
}

func (*transientFileSuite) assertFileExists(c *tc.C, path string) {
	stat, err := os.Stat(path)
	c.Assert(err, jc.ErrorIsNil, tc.Commentf("stat failed for %q", path))

	c.Assert(stat.IsDir(), jc.IsFalse, tc.Commentf("%q seems to be a directory", path))
}
