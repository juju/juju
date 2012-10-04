package uniter_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/version"
	"launchpad.net/juju-core/worker/uniter"
	"launchpad.net/juju-core/worker/uniter/jujuc"
	"os"
	"path/filepath"
	"time"
)

type ToolsSuite struct {
	dataDir, toolsDir string
}

var _ = Suite(&ToolsSuite{})

func (s *ToolsSuite) SetUpTest(c *C) {
	s.dataDir = c.MkDir()
	s.toolsDir = environs.ToolsDir(s.dataDir, version.Current)
	err := os.MkdirAll(s.toolsDir, 0755)
	c.Assert(err, IsNil)
	err = os.Symlink(s.toolsDir, environs.AgentToolsDir(s.dataDir, "unit-u-123"))
	c.Assert(err, IsNil)
}

func (s *ToolsSuite) TestEnsureJujucSymlinks(c *C) {
	jujucPath := filepath.Join(s.toolsDir, "jujuc")
	err := ioutil.WriteFile(jujucPath, []byte("assume sane"), 0755)
	c.Assert(err, IsNil)

	assertLink := func(path string) time.Time {
		target, err := os.Readlink(path)
		c.Assert(err, IsNil)
		c.Assert(target, Equals, "./jujuc")
		fi, err := os.Lstat(path)
		c.Assert(err, IsNil)
		return fi.ModTime()
	}

	// Check that EnsureJujucSymlinks writes appropriate symlinks.
	err = uniter.EnsureJujucSymlinks(s.toolsDir)
	c.Assert(err, IsNil)
	mtimes := map[string]time.Time{}
	for _, name := range jujuc.CommandNames() {
		tool := filepath.Join(s.toolsDir, name)
		mtimes[tool] = assertLink(tool)
	}

	// Check that EnsureJujucSymlinks doesn't overwrite things that don't need to be.
	err = uniter.EnsureJujucSymlinks(s.toolsDir)
	c.Assert(err, IsNil)
	for tool, mtime := range mtimes {
		c.Assert(assertLink(tool), Equals, mtime)
	}
}

func (s *ToolsSuite) TestEnsureJujucSymlinksBadDir(c *C) {
	err := uniter.EnsureJujucSymlinks(filepath.Join(c.MkDir(), "noexist"))
	c.Assert(err, ErrorMatches, "cannot initialize hook commands in .*: no such file or directory")
}
