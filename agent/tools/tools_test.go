// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	agenttools "github.com/juju/juju/agent/tools"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/testing"
	coretest "github.com/juju/juju/internal/tools"
)

type ToolsImportSuite struct {
}

var _ = tc.Suite(&ToolsImportSuite{})

func (t *ToolsImportSuite) TestPackageDependencies(c *tc.C) {
	// This test is to ensure we don't bring in dependencies on state, environ
	// or any of the other bigger packages that'll drag in yet more dependencies.
	// Only imports that start with "github.com/juju/juju" are checked, and the
	// resulting slice has that prefix removed to keep the output short.
	c.Assert(testing.FindJujuCoreImports(c, "github.com/juju/juju/agent/tools"),
		jc.SameContents,
		[]string{
			"core/credential",
			"core/errors",
			"core/life",
			"core/logger",
			"core/model",
			"core/permission",
			"core/semversion",
			"core/status",
			"core/trace",
			"core/user",
			"internal/errors",
			"internal/logger",
			"internal/tools",
			"internal/uuid",
			"juju/names",
		})
}

type ToolsSuite struct {
	testing.BaseSuite
	dataDir string
}

var _ = tc.Suite(&ToolsSuite{})

func (t *ToolsSuite) SetUpTest(c *tc.C) {
	t.BaseSuite.SetUpTest(c)
	t.dataDir = c.MkDir()
}

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
	initBadDataTest("../../etc/passwd", agenttools.DirPerm, "", "bad name.*"),
	initBadDataTest(`\ini.sys`, agenttools.DirPerm, "", "bad name.*"),
	{[]byte("x"), "2d711642b726b04401627ca9fbac32f5c8530fb1903cc4db02258717921a4881", "unexpected EOF"},
	{gzyesses, "8d900c68a1a847aae4e95edcb29fcecd142c9b88ca4fe63209c216edbed546e1", "archive/tar: invalid tar header"},
}

func (t *ToolsSuite) TestUnpackToolsBadData(c *tc.C) {
	for i, test := range unpackToolsBadDataTests {
		c.Logf("test %d", i)
		testTools := &coretest.Tools{
			URL:     "http://foo/bar",
			Version: semversion.MustParseBinary("1.2.3-ubuntu-amd64"),
			Size:    int64(len(test.data)),
			SHA256:  test.checksum,
		}
		err := agenttools.UnpackTools(t.dataDir, testTools, bytes.NewReader(test.data))
		c.Assert(err, tc.ErrorMatches, test.err)
		assertDirNames(c, t.toolsDir(), []string{})
	}
}

func (t *ToolsSuite) TestUnpackToolsBadChecksum(c *tc.C) {
	data, _ := testing.TarGz(testing.NewTarFile("tools", agenttools.DirPerm, "some data"))
	testTools := &coretest.Tools{
		URL:     "http://foo/bar",
		Version: semversion.MustParseBinary("1.2.3-ubuntu-amd64"),
		Size:    int64(len(data)),
		SHA256:  "1234",
	}
	err := agenttools.UnpackTools(t.dataDir, testTools, bytes.NewReader(data))
	c.Assert(err, tc.ErrorMatches, "tarball sha256 mismatch, expected 1234, got .*")
	_, err = os.Stat(t.toolsDir())
	c.Assert(err, tc.FitsTypeOf, &os.PathError{})
}

func (t *ToolsSuite) toolsDir() string {
	return filepath.Join(t.dataDir, "tools")
}

func (t *ToolsSuite) TestUnpackToolsContents(c *tc.C) {
	files := []*testing.TarFile{
		testing.NewTarFile("bar", agenttools.DirPerm, "bar contents"),
		testing.NewTarFile("foo", agenttools.DirPerm, "foo contents"),
	}
	data, checksum := testing.TarGz(files...)
	testTools := &coretest.Tools{
		URL:     "http://foo/bar",
		Version: semversion.MustParseBinary("1.2.3-ubuntu-amd64"),
		Size:    int64(len(data)),
		SHA256:  checksum,
	}

	err := agenttools.UnpackTools(t.dataDir, testTools, bytes.NewReader(data))
	c.Assert(err, jc.ErrorIsNil)
	assertDirNames(c, t.toolsDir(), []string{"1.2.3-ubuntu-amd64"})
	t.assertToolsContents(c, testTools, files)

	// Try to unpack the same version of tools again - it should succeed,
	// leaving the original version around.
	files2 := []*testing.TarFile{
		testing.NewTarFile("bar", agenttools.DirPerm, "bar2 contents"),
		testing.NewTarFile("x", agenttools.DirPerm, "x contents"),
	}
	data2, checksum2 := testing.TarGz(files2...)
	tools2 := &coretest.Tools{
		URL:     "http://arble",
		Version: semversion.MustParseBinary("1.2.3-ubuntu-amd64"),
		Size:    int64(len(data2)),
		SHA256:  checksum2,
	}
	err = agenttools.UnpackTools(t.dataDir, tools2, bytes.NewReader(data2))
	c.Assert(err, jc.ErrorIsNil)
	assertDirNames(c, t.toolsDir(), []string{"1.2.3-ubuntu-amd64"})
	t.assertToolsContents(c, testTools, files)
}

func (t *ToolsSuite) TestReadToolsErrors(c *tc.C) {
	vers := semversion.MustParseBinary("1.2.3-ubuntu-amd64")
	testTools, err := agenttools.ReadTools(t.dataDir, vers)
	c.Assert(testTools, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "cannot read agent metadata in directory .*")

	dir := agenttools.SharedToolsDir(t.dataDir, vers)
	err = os.MkdirAll(dir, agenttools.DirPerm)
	c.Assert(err, jc.ErrorIsNil)

	err = os.WriteFile(filepath.Join(dir, agenttools.ToolsFile), []byte(" \t\n"), 0644)
	c.Assert(err, jc.ErrorIsNil)

	testTools, err = agenttools.ReadTools(t.dataDir, vers)
	c.Assert(testTools, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "invalid agent metadata in directory .*")
}

func (t *ToolsSuite) TestChangeAgentTools(c *tc.C) {
	files := []*testing.TarFile{
		testing.NewTarFile("jujuc", agenttools.DirPerm, "juju executable"),
		testing.NewTarFile("jujud", agenttools.DirPerm, "jujuc executable"),
	}
	data, checksum := testing.TarGz(files...)
	testTools := &coretest.Tools{
		URL:     "http://foo/bar1",
		Version: semversion.MustParseBinary("1.2.3-ubuntu-amd64"),
		Size:    int64(len(data)),
		SHA256:  checksum,
	}
	err := agenttools.UnpackTools(t.dataDir, testTools, bytes.NewReader(data))
	c.Assert(err, jc.ErrorIsNil)

	gotTools, err := agenttools.ChangeAgentTools(t.dataDir, "testagent", testTools.Version)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*gotTools, tc.Equals, *testTools)

	assertDirNames(c, t.toolsDir(), []string{"1.2.3-ubuntu-amd64", "testagent"})
	assertDirNames(c, agenttools.ToolsDir(t.dataDir, "testagent"), []string{"jujuc", "jujud", agenttools.ToolsFile})

	// Upgrade again to check that the link replacement logic works ok.
	files2 := []*testing.TarFile{
		testing.NewTarFile("ubuntu", agenttools.DirPerm, "foo content"),
		testing.NewTarFile("amd64", agenttools.DirPerm, "bar content"),
	}
	data2, checksum2 := testing.TarGz(files2...)
	tools2 := &coretest.Tools{
		URL:     "http://foo/bar2",
		Version: semversion.MustParseBinary("1.2.4-ubuntu-amd64"),
		Size:    int64(len(data2)),
		SHA256:  checksum2,
	}
	err = agenttools.UnpackTools(t.dataDir, tools2, bytes.NewReader(data2))
	c.Assert(err, jc.ErrorIsNil)

	gotTools, err = agenttools.ChangeAgentTools(t.dataDir, "testagent", tools2.Version)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*gotTools, tc.Equals, *tools2)

	assertDirNames(c, t.toolsDir(), []string{"1.2.3-ubuntu-amd64", "1.2.4-ubuntu-amd64", "testagent"})
	assertDirNames(c, agenttools.ToolsDir(t.dataDir, "testagent"), []string{"ubuntu", "amd64", agenttools.ToolsFile})
}

func (t *ToolsSuite) TestSharedToolsDir(c *tc.C) {
	dir := agenttools.SharedToolsDir("/var/lib/juju", semversion.MustParseBinary("1.2.3-ubuntu-amd64"))
	c.Assert(dir, tc.Equals, "/var/lib/juju/tools/1.2.3-ubuntu-amd64")
}

// assertToolsContents asserts that the directory for the tools
// has the given contents.
func (t *ToolsSuite) assertToolsContents(c *tc.C, testTools *coretest.Tools, files []*testing.TarFile) {
	var wantNames []string
	for _, f := range files {
		wantNames = append(wantNames, f.Header.Name)
	}
	wantNames = append(wantNames, agenttools.ToolsFile)
	dir := agenttools.SharedToolsDir(t.dataDir, testTools.Version)
	assertDirNames(c, dir, wantNames)
	expectedURLFileContents, err := json.Marshal(testTools)
	c.Assert(err, jc.ErrorIsNil)
	assertFileContents(c, dir, agenttools.ToolsFile, string(expectedURLFileContents), 0200)
	for _, f := range files {
		assertFileContents(c, dir, f.Header.Name, f.Contents, 0400)
	}
	gotTools, err := agenttools.ReadTools(t.dataDir, testTools.Version)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*gotTools, tc.Equals, *testTools)
}

// assertFileContents asserts that the given file in the
// given directory has the given contents.
func assertFileContents(c *tc.C, dir, file, contents string, mode os.FileMode) {
	file = filepath.Join(dir, file)
	info, err := os.Stat(file)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.Mode()&(os.ModeType|mode), tc.Equals, mode)
	data, err := os.ReadFile(file)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), tc.Equals, contents)
}

// assertDirNames asserts that the given directory
// holds the given file or directory names.
func assertDirNames(c *tc.C, dir string, names []string) {
	f, err := os.Open(dir)
	c.Assert(err, jc.ErrorIsNil)
	defer f.Close()
	dnames, err := f.Readdirnames(0)
	c.Assert(err, jc.ErrorIsNil)
	sort.Strings(dnames)
	sort.Strings(names)
	c.Assert(dnames, tc.DeepEquals, names)
}
