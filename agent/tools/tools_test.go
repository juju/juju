// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"

	gc "launchpad.net/gocheck"

	agenttools "launchpad.net/juju-core/agent/tools"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/testbase"
	coretest "launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/version"
)

type ToolsSuite struct {
	testing.BaseSuite
	dataDir string
}

var _ = gc.Suite(&ToolsSuite{})

func (t *ToolsSuite) SetUpTest(c *gc.C) {
	t.BaseSuite.SetUpTest(c)
	t.dataDir = c.MkDir()
}

func (t *ToolsSuite) TestPackageDependencies(c *gc.C) {
	// This test is to ensure we don't bring in dependencies on state, environ
	// or any of the other bigger packages that'll drag in yet more dependencies.
	// Only imports that start with "launchpad.net/juju-core" are checked, and the
	// resulting slice has that prefix removed to keep the output short.
	c.Assert(testbase.FindJujuCoreImports(c, "launchpad.net/juju-core/agent/tools"),
		gc.DeepEquals,
		[]string{"juju/arch", "tools", "utils/set", "version"})
}

const toolsFile = "downloaded-tools.txt"

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

type badDataTest struct {
	data     []byte
	checksum string
	err      string
}

func initBadDataTest(name string, mode os.FileMode, contents string, err string) badDataTest {
	var result badDataTest
	result.data, result.checksum = testing.TarGz(testing.NewTarFile(name, mode, contents))
	result.err = err
	return result
}

var unpackToolsBadDataTests = []badDataTest{
	initBadDataTest("bar", os.ModeDir, "", "bad file type.*"),
	initBadDataTest("../../etc/passwd", 0755, "", "bad name.*"),
	initBadDataTest(`\ini.sys`, 0755, "", "bad name.*"),
	badDataTest{[]byte("x"), "2d711642b726b04401627ca9fbac32f5c8530fb1903cc4db02258717921a4881", "unexpected EOF"},
	badDataTest{gzyesses, "8d900c68a1a847aae4e95edcb29fcecd142c9b88ca4fe63209c216edbed546e1", "archive/tar: invalid tar header"},
}

func (t *ToolsSuite) TestUnpackToolsBadData(c *gc.C) {
	for i, test := range unpackToolsBadDataTests {
		c.Logf("test %d", i)
		testTools := &coretest.Tools{
			URL:     "http://foo/bar",
			Version: version.MustParseBinary("1.2.3-foo-bar"),
			Size:    int64(len(test.data)),
			SHA256:  test.checksum,
		}
		err := agenttools.UnpackTools(t.dataDir, testTools, bytes.NewReader(test.data))
		c.Assert(err, gc.ErrorMatches, test.err)
		assertDirNames(c, t.toolsDir(), []string{})
	}
}

func (t *ToolsSuite) TestUnpackToolsBadChecksum(c *gc.C) {
	data, _ := testing.TarGz(testing.NewTarFile("tools", 0755, "some data"))
	testTools := &coretest.Tools{
		URL:     "http://foo/bar",
		Version: version.MustParseBinary("1.2.3-foo-bar"),
		Size:    int64(len(data)),
		SHA256:  "1234",
	}
	err := agenttools.UnpackTools(t.dataDir, testTools, bytes.NewReader(data))
	c.Assert(err, gc.ErrorMatches, "tarball sha256 mismatch, expected 1234, got .*")
	_, err = os.Stat(t.toolsDir())
	c.Assert(err, gc.FitsTypeOf, &os.PathError{})
}

func (t *ToolsSuite) toolsDir() string {
	return filepath.Join(t.dataDir, "tools")
}

func (t *ToolsSuite) TestUnpackToolsContents(c *gc.C) {
	files := []*testing.TarFile{
		testing.NewTarFile("bar", 0755, "bar contents"),
		testing.NewTarFile("foo", 0755, "foo contents"),
	}
	data, checksum := testing.TarGz(files...)
	testTools := &coretest.Tools{
		URL:     "http://foo/bar",
		Version: version.MustParseBinary("1.2.3-foo-bar"),
		Size:    int64(len(data)),
		SHA256:  checksum,
	}

	err := agenttools.UnpackTools(t.dataDir, testTools, bytes.NewReader(data))
	c.Assert(err, gc.IsNil)
	assertDirNames(c, t.toolsDir(), []string{"1.2.3-foo-bar"})
	t.assertToolsContents(c, testTools, files)

	// Try to unpack the same version of tools again - it should succeed,
	// leaving the original version around.
	files2 := []*testing.TarFile{
		testing.NewTarFile("bar", 0755, "bar2 contents"),
		testing.NewTarFile("x", 0755, "x contents"),
	}
	data2, checksum2 := testing.TarGz(files2...)
	tools2 := &coretest.Tools{
		URL:     "http://arble",
		Version: version.MustParseBinary("1.2.3-foo-bar"),
		Size:    int64(len(data2)),
		SHA256:  checksum2,
	}
	err = agenttools.UnpackTools(t.dataDir, tools2, bytes.NewReader(data2))
	c.Assert(err, gc.IsNil)
	assertDirNames(c, t.toolsDir(), []string{"1.2.3-foo-bar"})
	t.assertToolsContents(c, testTools, files)
}

func (t *ToolsSuite) TestReadToolsErrors(c *gc.C) {
	vers := version.MustParseBinary("1.2.3-precise-amd64")
	testTools, err := agenttools.ReadTools(t.dataDir, vers)
	c.Assert(testTools, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "cannot read tools metadata in tools directory: .*")

	dir := agenttools.SharedToolsDir(t.dataDir, vers)
	err = os.MkdirAll(dir, 0755)
	c.Assert(err, gc.IsNil)

	err = ioutil.WriteFile(filepath.Join(dir, toolsFile), []byte(" \t\n"), 0644)
	c.Assert(err, gc.IsNil)

	testTools, err = agenttools.ReadTools(t.dataDir, vers)
	c.Assert(testTools, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "invalid tools metadata in tools directory .*")
}

func (t *ToolsSuite) TestChangeAgentTools(c *gc.C) {
	files := []*testing.TarFile{
		testing.NewTarFile("jujuc", 0755, "juju executable"),
		testing.NewTarFile("jujud", 0755, "jujuc executable"),
	}
	data, checksum := testing.TarGz(files...)
	testTools := &coretest.Tools{
		URL:     "http://foo/bar1",
		Version: version.MustParseBinary("1.2.3-foo-bar"),
		Size:    int64(len(data)),
		SHA256:  checksum,
	}
	err := agenttools.UnpackTools(t.dataDir, testTools, bytes.NewReader(data))
	c.Assert(err, gc.IsNil)

	gotTools, err := agenttools.ChangeAgentTools(t.dataDir, "testagent", testTools.Version)
	c.Assert(err, gc.IsNil)
	c.Assert(*gotTools, gc.Equals, *testTools)

	assertDirNames(c, t.toolsDir(), []string{"1.2.3-foo-bar", "testagent"})
	assertDirNames(c, agenttools.ToolsDir(t.dataDir, "testagent"), []string{"jujuc", "jujud", toolsFile})

	// Upgrade again to check that the link replacement logic works ok.
	files2 := []*testing.TarFile{
		testing.NewTarFile("foo", 0755, "foo content"),
		testing.NewTarFile("bar", 0755, "bar content"),
	}
	data2, checksum2 := testing.TarGz(files2...)
	tools2 := &coretest.Tools{
		URL:     "http://foo/bar2",
		Version: version.MustParseBinary("1.2.4-foo-bar"),
		Size:    int64(len(data2)),
		SHA256:  checksum2,
	}
	err = agenttools.UnpackTools(t.dataDir, tools2, bytes.NewReader(data2))
	c.Assert(err, gc.IsNil)

	gotTools, err = agenttools.ChangeAgentTools(t.dataDir, "testagent", tools2.Version)
	c.Assert(err, gc.IsNil)
	c.Assert(*gotTools, gc.Equals, *tools2)

	assertDirNames(c, t.toolsDir(), []string{"1.2.3-foo-bar", "1.2.4-foo-bar", "testagent"})
	assertDirNames(c, agenttools.ToolsDir(t.dataDir, "testagent"), []string{"foo", "bar", toolsFile})
}

func (t *ToolsSuite) TestSharedToolsDir(c *gc.C) {
	dir := agenttools.SharedToolsDir("/var/lib/juju", version.MustParseBinary("1.2.3-precise-amd64"))
	c.Assert(dir, gc.Equals, "/var/lib/juju/tools/1.2.3-precise-amd64")
}

// assertToolsContents asserts that the directory for the tools
// has the given contents.
func (t *ToolsSuite) assertToolsContents(c *gc.C, testTools *coretest.Tools, files []*testing.TarFile) {
	var wantNames []string
	for _, f := range files {
		wantNames = append(wantNames, f.Header.Name)
	}
	wantNames = append(wantNames, toolsFile)
	dir := agenttools.SharedToolsDir(t.dataDir, testTools.Version)
	assertDirNames(c, dir, wantNames)
	expectedURLFileContents, err := json.Marshal(testTools)
	c.Assert(err, gc.IsNil)
	assertFileContents(c, dir, toolsFile, string(expectedURLFileContents), 0200)
	for _, f := range files {
		assertFileContents(c, dir, f.Header.Name, f.Contents, 0400)
	}
	gotTools, err := agenttools.ReadTools(t.dataDir, testTools.Version)
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
