// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/series"
	"github.com/juju/utils/symlink"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent/tools"
	"github.com/juju/juju/juju/names"
	jujuversion "github.com/juju/juju/version"
)

type SymlinksSuite struct {
	dataDir, toolsDir string
}

var _ = gc.Suite(&SymlinksSuite{})

func (s *SymlinksSuite) SetUpTest(c *gc.C) {
	s.dataDir = c.MkDir()
	s.toolsDir = tools.SharedToolsDir(s.dataDir, version.Binary{
		Number: jujuversion.Current,
		Arch:   arch.HostArch(),
		Series: series.MustHostSeries(),
	})
	err := os.MkdirAll(s.toolsDir, 0755)
	c.Assert(err, jc.ErrorIsNil)
	err = symlink.New(s.toolsDir, tools.ToolsDir(s.dataDir, "unit-u-123"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SymlinksSuite) TestEnsureSymlinks(c *gc.C) {
	s.testEnsureSymlinks(c, s.toolsDir)
}

func (s *SymlinksSuite) TestEnsureSymlinksSymlinkedDir(c *gc.C) {
	dirSymlink := filepath.Join(c.MkDir(), "commands")
	err := symlink.New(s.toolsDir, dirSymlink)
	c.Assert(err, jc.ErrorIsNil)
	s.testEnsureSymlinks(c, dirSymlink)
}

func (s *SymlinksSuite) testEnsureSymlinks(c *gc.C, dir string) {
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

	commands := []string{"foo", "bar"}

	// Check that EnsureSymlinks writes appropriate symlinks.
	err = tools.EnsureSymlinks(dir, dir, commands)
	c.Assert(err, jc.ErrorIsNil)
	mtimes := map[string]time.Time{}
	for _, name := range commands {
		tool := filepath.Join(s.toolsDir, name)
		mtimes[tool] = assertLink(tool)
	}

	// Check that EnsureSymlinks doesn't overwrite things that don't need to be.
	err = tools.EnsureSymlinks(s.toolsDir, s.toolsDir, commands)
	c.Assert(err, jc.ErrorIsNil)
	for tool, mtime := range mtimes {
		c.Assert(assertLink(tool), gc.Equals, mtime)
	}
}

func (s *SymlinksSuite) TestEnsureSymlinksBadDir(c *gc.C) {
	dir := filepath.Join(c.MkDir(), "noexist")
	err := tools.EnsureSymlinks(dir, dir, []string{"foo"})
	c.Assert(err, gc.ErrorMatches, "cannot initialize commands in .*: "+utils.NoSuchFileErrRegexp)
}
