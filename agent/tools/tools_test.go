// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/agent/tools"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
)

type ToolsSuite struct {
	testing.LoggingSuite
	dataDir string
}

var _ = gc.Suite(&ToolsSuite{})

func (t *ToolsSuite) SetUpTest(c *gc.C) {
	t.LoggingSuite.SetUpTest(c)
	t.dataDir = c.MkDir()
}

func (t *ToolsSuite) TestPackageDependencies(c *gc.C) {
	c.Assert(testing.FindJujuCoreImports(c, "launchpad.net/juju-core/agent/tools"),
		gc.DeepEquals,
		[]string{"utils/set", "version"})
}

const urlFile = "downloaded-url.txt"

// gzyesses holds the result of running:
// yes | head -17000 | gzip
var gzyesses = []byte{
	0x1f, 0x8b, 0x08, 0x00, 0x29, 0xae, 0x1a, 0x50,
	0x00, 0x03, 0xed, 0xc2, 0x31, 0x0d, 0x00, 0x00,
	0x00, 0x02, 0xa0, 0xdf, 0xc6, 0xb6, 0xb7, 0x87,
	0x63, 0xd0, 0x14, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x38, 0x31, 0x53, 0xad, 0x03,
	0x8d, 0xd0, 0x84, 0x00, 0x00,
}

var unpackToolsBadDataTests = []struct {
	data []byte
	err  string
}{
	{
		testing.TarGz(testing.NewTarFile("bar", os.ModeDir, "")),
		"bad file type.*",
	}, {
		testing.TarGz(testing.NewTarFile("../../etc/passwd", 0755, "")),
		"bad name.*",
	}, {
		testing.TarGz(testing.NewTarFile(`\ini.sys`, 0755, "")),
		"bad name.*",
	}, {
		[]byte("x"),
		"unexpected EOF",
	}, {
		gzyesses,
		"archive/tar: invalid tar header",
	},
}

func (t *ToolsSuite) TestUnpackToolsBadData(c *gc.C) {
	for i, test := range unpackToolsBadDataTests {
		c.Logf("test %d", i)
		testTools := &tools.Tools{
			URL:    "http://foo/bar",
			Binary: version.MustParseBinary("1.2.3-foo-bar"),
		}
		err := tools.UnpackTools(t.dataDir, testTools, bytes.NewReader(test.data))
		c.Assert(err, gc.ErrorMatches, test.err)
		assertDirNames(c, t.toolsDir(), []string{})
	}
}

func (t *ToolsSuite) toolsDir() string {
	return filepath.Join(t.dataDir, "tools")
}

func (t *ToolsSuite) TestUnpackToolsContents(c *gc.C) {
	files := []*testing.TarFile{
		testing.NewTarFile("bar", 0755, "bar contents"),
		testing.NewTarFile("foo", 0755, "foo contents"),
	}
	testTools := &tools.Tools{
		URL:    "http://foo/bar",
		Binary: version.MustParseBinary("1.2.3-foo-bar"),
	}

	err := tools.UnpackTools(t.dataDir, testTools, bytes.NewReader(testing.TarGz(files...)))
	c.Assert(err, gc.IsNil)
	assertDirNames(c, t.toolsDir(), []string{"1.2.3-foo-bar"})
	t.assertToolsContents(c, testTools, files)

	// Try to unpack the same version of tools again - it should succeed,
	// leaving the original version around.
	tools2 := &tools.Tools{
		URL:    "http://arble",
		Binary: version.MustParseBinary("1.2.3-foo-bar"),
	}
	files2 := []*testing.TarFile{
		testing.NewTarFile("bar", 0755, "bar2 contents"),
		testing.NewTarFile("x", 0755, "x contents"),
	}
	err = tools.UnpackTools(t.dataDir, tools2, bytes.NewReader(testing.TarGz(files2...)))
	c.Assert(err, gc.IsNil)
	assertDirNames(c, t.toolsDir(), []string{"1.2.3-foo-bar"})
	t.assertToolsContents(c, testTools, files)
}

func (t *ToolsSuite) TestReadToolsErrors(c *gc.C) {
	vers := version.MustParseBinary("1.2.3-precise-amd64")
	testTools, err := tools.ReadTools(t.dataDir, vers)
	c.Assert(testTools, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "cannot read URL in tools directory: .*")

	dir := tools.SharedToolsDir(t.dataDir, vers)
	err = os.MkdirAll(dir, 0755)
	c.Assert(err, gc.IsNil)

	err = ioutil.WriteFile(filepath.Join(dir, urlFile), []byte(" \t\n"), 0644)
	c.Assert(err, gc.IsNil)

	testTools, err = tools.ReadTools(t.dataDir, vers)
	c.Assert(testTools, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "empty URL in tools directory.*")
}

func (t *ToolsSuite) TestChangeAgentTools(c *gc.C) {
	files := []*testing.TarFile{
		testing.NewTarFile("jujuc", 0755, "juju executable"),
		testing.NewTarFile("jujud", 0755, "jujuc executable"),
	}
	testTools := &tools.Tools{
		URL:    "http://foo/bar1",
		Binary: version.MustParseBinary("1.2.3-foo-bar"),
	}
	err := tools.UnpackTools(t.dataDir, testTools, bytes.NewReader(testing.TarGz(files...)))
	c.Assert(err, gc.IsNil)

	gotTools, err := tools.ChangeAgentTools(t.dataDir, "testagent", testTools.Binary)
	c.Assert(err, gc.IsNil)
	c.Assert(*gotTools, gc.Equals, *testTools)

	assertDirNames(c, t.toolsDir(), []string{"1.2.3-foo-bar", "testagent"})
	assertDirNames(c, tools.ToolsDir(t.dataDir, "testagent"), []string{"jujuc", "jujud", urlFile})

	// Upgrade again to check that the link replacement logic works ok.
	files2 := []*testing.TarFile{
		testing.NewTarFile("foo", 0755, "foo content"),
		testing.NewTarFile("bar", 0755, "bar content"),
	}
	tools2 := &tools.Tools{
		URL:    "http://foo/bar2",
		Binary: version.MustParseBinary("1.2.4-foo-bar"),
	}
	err = tools.UnpackTools(t.dataDir, tools2, bytes.NewReader(testing.TarGz(files2...)))
	c.Assert(err, gc.IsNil)

	gotTools, err = tools.ChangeAgentTools(t.dataDir, "testagent", tools2.Binary)
	c.Assert(err, gc.IsNil)
	c.Assert(*gotTools, gc.Equals, *tools2)

	assertDirNames(c, t.toolsDir(), []string{"1.2.3-foo-bar", "1.2.4-foo-bar", "testagent"})
	assertDirNames(c, tools.ToolsDir(t.dataDir, "testagent"), []string{"foo", "bar", urlFile})
}

func (t *ToolsSuite) TestSharedToolsDir(c *gc.C) {
	dir := tools.SharedToolsDir("/var/lib/juju", version.MustParseBinary("1.2.3-precise-amd64"))
	c.Assert(dir, gc.Equals, "/var/lib/juju/tools/1.2.3-precise-amd64")
}

// assertToolsContents asserts that the directory for the tools
// has the given contents.
func (t *ToolsSuite) assertToolsContents(c *gc.C, testTools *tools.Tools, files []*testing.TarFile) {
	var wantNames []string
	for _, f := range files {
		wantNames = append(wantNames, f.Header.Name)
	}
	wantNames = append(wantNames, urlFile)
	dir := tools.SharedToolsDir(t.dataDir, testTools.Binary)
	assertDirNames(c, dir, wantNames)
	assertFileContents(c, dir, urlFile, testTools.URL, 0200)
	for _, f := range files {
		assertFileContents(c, dir, f.Header.Name, f.Contents, 0400)
	}
	gotTools, err := tools.ReadTools(t.dataDir, testTools.Binary)
	c.Assert(err, gc.IsNil)
	c.Assert(*gotTools, gc.Equals, *testTools)
}

// assertFileContents asserts that the given file in the
// given directory has the given contents.
func assertFileContents(c *gc.C, dir, file, contents string, mode os.FileMode) {
	file = filepath.Join(dir, file)
	info, err := os.Stat(file)
	c.Assert(err, gc.IsNil)
	c.Assert(info.Mode()&(os.ModeType|mode), gc.Equals, mode)
	data, err := ioutil.ReadFile(file)
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Equals, contents)
}

// assertDirNames asserts that the given directory
// holds the given file or directory names.
func assertDirNames(c *gc.C, dir string, names []string) {
	f, err := os.Open(dir)
	c.Assert(err, gc.IsNil)
	defer f.Close()
	dnames, err := f.Readdirnames(0)
	c.Assert(err, gc.IsNil)
	sort.Strings(dnames)
	sort.Strings(names)
	c.Assert(dnames, gc.DeepEquals, names)
}
