// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/utils/symlink"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/agent/tools"
	"github.com/juju/juju/juju/names"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker/uniter"
	"github.com/juju/juju/worker/uniter/jujuc"
)

type ToolsSuite struct {
	dataDir, toolsDir string
}

var _ = gc.Suite(&ToolsSuite{})

func (s *ToolsSuite) SetUpTest(c *gc.C) {
	s.dataDir = c.MkDir()
	s.toolsDir = tools.SharedToolsDir(s.dataDir, version.Current)
	err := os.MkdirAll(s.toolsDir, 0755)
	c.Assert(err, gc.IsNil)
	err = symlink.New(s.toolsDir, tools.ToolsDir(s.dataDir, "unit-u-123"))
	c.Assert(err, gc.IsNil)
}

func (s *ToolsSuite) TestEnsureJujucSymlinks(c *gc.C) {
	jujudPath := filepath.Join(s.toolsDir, names.Jujud)
	err := ioutil.WriteFile(jujudPath, []byte("assume sane"), 0755)
	c.Assert(err, gc.IsNil)

	assertLink := func(path string) time.Time {
		target, err := symlink.Read(path)
		c.Assert(err, gc.IsNil)
		c.Assert(target, gc.Equals, jujudPath)
		fi, err := os.Lstat(path)
		c.Assert(err, gc.IsNil)
		return fi.ModTime()
	}

	// Check that EnsureJujucSymlinks writes appropriate symlinks.
	err = uniter.EnsureJujucSymlinks(s.toolsDir)
	c.Assert(err, gc.IsNil)
	mtimes := map[string]time.Time{}
	for _, name := range jujuc.CommandNames() {
		tool := filepath.Join(s.toolsDir, name)
		mtimes[tool] = assertLink(tool)
	}

	// Check that EnsureJujucSymlinks doesn't overwrite things that don't need to be.
	err = uniter.EnsureJujucSymlinks(s.toolsDir)
	c.Assert(err, gc.IsNil)
	for tool, mtime := range mtimes {
		c.Assert(assertLink(tool), gc.Equals, mtime)
	}
}

func (s *ToolsSuite) TestEnsureJujucSymlinksBadDir(c *gc.C) {
	err := uniter.EnsureJujucSymlinks(filepath.Join(c.MkDir(), "noexist"))
	c.Assert(err, gc.ErrorMatches, "cannot initialize hook commands in .*: no such file or directory")
}
