// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build !windows

package transientfile

import (
	"os"
	"path/filepath"
	stdtesting "testing"

	"github.com/juju/tc"
)

type transientFileSuite struct{}

func TestTransientFileSuite(t *stdtesting.T) { tc.Run(t, &transientFileSuite{}) }
func (s *transientFileSuite) TestCreateTransientFile(c *tc.C) {
	transientDir := c.MkDir()
	f, err := Create(transientDir, "foo.test")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(f.Close(), tc.ErrorIsNil)

	expPath := filepath.Join(transientDir, "foo.test")
	s.assertFileExists(c, expPath)
}

func (s *transientFileSuite) TestCreateTransientFileInSubdirectory(c *tc.C) {
	transientDir := c.MkDir()
	f, err := Create(transientDir, filepath.Join("1", "2", "foo.test"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(f.Close(), tc.ErrorIsNil)

	expPath := filepath.Join(transientDir, "1", "2", "foo.test")
	s.assertFileExists(c, expPath)
}

func (*transientFileSuite) assertFileExists(c *tc.C, path string) {
	stat, err := os.Stat(path)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("stat failed for %q", path))

	c.Assert(stat.IsDir(), tc.IsFalse, tc.Commentf("%q seems to be a directory", path))
}
