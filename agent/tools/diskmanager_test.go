// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"

	gc "gopkg.in/check.v1"

	agenttools "github.com/juju/juju/agent/tools"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

var _ = gc.Suite(&DiskManagerSuite{})

var _ agenttools.ToolsManager = (*agenttools.DiskManager)(nil)

type DiskManagerSuite struct {
	coretesting.BaseSuite
	dataDir string
	manager agenttools.ToolsManager
}

func (s *DiskManagerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.dataDir = c.MkDir()
	s.manager = agenttools.NewDiskManager(s.dataDir)
}

func (s *DiskManagerSuite) toolsDir() string {
	// TODO: Somehow hide this behind the DataManager
	return filepath.Join(s.dataDir, "tools")
}

// Copied from environs/agent/tools_test.go
func (s *DiskManagerSuite) TestUnpackToolsContents(c *gc.C) {
	files := []*coretesting.TarFile{
		coretesting.NewTarFile("amd64", agenttools.DirPerm, "bar contents"),
		coretesting.NewTarFile("quantal", agenttools.DirPerm, "foo contents"),
	}
	gzfile, checksum := coretesting.TarGz(files...)
	t1 := &coretools.Tools{
		URL:     "http://foo/bar",
		Version: version.MustParseBinary("1.2.3-quantal-amd64"),
		Size:    int64(len(gzfile)),
		SHA256:  checksum,
	}

	err := s.manager.UnpackTools(t1, bytes.NewReader(gzfile))
	c.Assert(err, gc.IsNil)
	assertDirNames(c, s.toolsDir(), []string{"1.2.3-quantal-amd64"})
	s.assertToolsContents(c, t1, files)

	// Try to unpack the same version of tools again - it should succeed,
	// leaving the original version around.
	files2 := []*coretesting.TarFile{
		coretesting.NewTarFile("bar", agenttools.DirPerm, "bar2 contents"),
		coretesting.NewTarFile("x", agenttools.DirPerm, "x contents"),
	}
	gzfile2, checksum2 := coretesting.TarGz(files2...)
	t2 := &coretools.Tools{
		URL:     "http://arble",
		Version: version.MustParseBinary("1.2.3-quantal-amd64"),
		Size:    int64(len(gzfile2)),
		SHA256:  checksum2,
	}
	err = s.manager.UnpackTools(t2, bytes.NewReader(gzfile2))
	c.Assert(err, gc.IsNil)
	assertDirNames(c, s.toolsDir(), []string{"1.2.3-quantal-amd64"})
	s.assertToolsContents(c, t1, files)
}

func (t *DiskManagerSuite) TestSharedToolsDir(c *gc.C) {
	manager := agenttools.NewDiskManager("/var/lib/juju")
	dir := manager.SharedToolsDir(version.MustParseBinary("1.2.3-precise-amd64"))
	c.Assert(dir, gc.Equals, "/var/lib/juju/tools/1.2.3-precise-amd64")
}

// assertToolsContents asserts that the directory for the tools
// has the given contents.
func (s *DiskManagerSuite) assertToolsContents(c *gc.C, t *coretools.Tools, files []*coretesting.TarFile) {
	var wantNames []string
	for _, f := range files {
		wantNames = append(wantNames, f.Header.Name)
	}
	wantNames = append(wantNames, toolsFile)
	dir := s.manager.(*agenttools.DiskManager).SharedToolsDir(t.Version)
	assertDirNames(c, dir, wantNames)
	expectedFileContents, err := json.Marshal(t)
	c.Assert(err, gc.IsNil)
	assertFileContents(c, dir, toolsFile, string(expectedFileContents), 0200)
	for _, f := range files {
		assertFileContents(c, dir, f.Header.Name, f.Contents, 0400)
	}
	gotTools, err := s.manager.ReadTools(t.Version)
	c.Assert(err, gc.IsNil)
	c.Assert(*gotTools, gc.Equals, *t)
	// Make sure that the tools directory is readable by the ubuntu user (for
	// juju-run)
	info, err := os.Stat(dir)
	c.Assert(err, gc.IsNil)
	c.Assert(info.Mode().Perm(), gc.Equals, os.FileMode(agenttools.DirPerm))
}
