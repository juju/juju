// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/symlink"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent/tools"
	"github.com/juju/juju/juju/names"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type ToolsSuite struct {
	dataDir, toolsDir string
}

var _ = gc.Suite(&ToolsSuite{})

func (s *ToolsSuite) SetUpTest(c *gc.C) {
	s.dataDir = c.MkDir()
	s.toolsDir = tools.SharedToolsDir(s.dataDir, version.Current)
	err := os.MkdirAll(s.toolsDir, 0755)
	c.Assert(err, jc.ErrorIsNil)
	err = symlink.New(s.toolsDir, tools.ToolsDir(s.dataDir, "unit-u-123"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ToolsSuite) TestEnsureSymlinks(c *gc.C) {
	s.testEnsureSymlinks(c, s.toolsDir)
}

func (s *ToolsSuite) TestEnsureSymlinksSymlinkedDir(c *gc.C) {
	toolsDirSymlink := filepath.Join(c.MkDir(), "unit-ubuntu-0")
	err := symlink.New(s.toolsDir, toolsDirSymlink)
	c.Assert(err, jc.ErrorIsNil)
	s.testEnsureSymlinks(c, toolsDirSymlink)
}

func (s *ToolsSuite) testEnsureSymlinks(c *gc.C, dir string) {
	jujudPath := filepath.Join(s.toolsDir, names.Jujud)
	err := ioutil.WriteFile(jujudPath, []byte("assume sane"), 0755)
	c.Assert(err, jc.ErrorIsNil)

	assertLink := func(path string) time.Time {
		target, err := symlink.Read(path)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(target, jc.SamePath, jujudPath)
		fi, err := os.Lstat(path)
		c.Assert(err, jc.ErrorIsNil)
		return fi.ModTime()
	}

	// Check that EnsureSymlinks writes appropriate symlinks.
	err = jujuc.EnsureSymlinks(dir)
	c.Assert(err, jc.ErrorIsNil)
	mtimes := map[string]time.Time{}
	for _, name := range jujuc.CommandNames() {
		tool := filepath.Join(s.toolsDir, name)
		mtimes[tool] = assertLink(tool)
	}

	// Check that EnsureSymlinks doesn't overwrite things that don't need to be.
	err = jujuc.EnsureSymlinks(s.toolsDir)
	c.Assert(err, jc.ErrorIsNil)
	for tool, mtime := range mtimes {
		c.Assert(assertLink(tool), gc.Equals, mtime)
	}
}

func (s *ToolsSuite) TestEnsureSymlinksBadDir(c *gc.C) {
	err := jujuc.EnsureSymlinks(filepath.Join(c.MkDir(), "noexist"))
	c.Assert(err, gc.ErrorMatches, "cannot initialize hook commands in .*: "+utils.NoSuchFileErrRegexp)
}
