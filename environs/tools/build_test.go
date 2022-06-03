// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	exttest "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3/arch"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	agenttools "github.com/juju/juju/agent/tools"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/juju/names"
	"github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
)

type buildSuite struct {
	testing.BaseSuite
	restore  func()
	cwd      string
	filePath string
	exttest.PatchExecHelper
}

var _ = gc.Suite(&buildSuite{})

func (b *buildSuite) SetUpTest(c *gc.C) {
	b.BaseSuite.SetUpTest(c)

	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".bat"
	}

	dir1 := c.MkDir()
	dir2 := c.MkDir()

	// Ensure we don't look in the real /usr/lib/juju for jujud-versions.yaml.
	b.PatchValue(&tools.VersionFileFallbackDir, c.MkDir())

	c.Log(dir1)
	c.Log(dir2)

	path := os.Getenv("PATH")
	os.Setenv("PATH", strings.Join([]string{dir1, dir2, path}, string(filepath.ListSeparator)))

	// Make an executable file called "juju-test" in dir2.
	b.filePath = filepath.Join(dir2, "juju-test"+suffix)
	err := ioutil.WriteFile(
		b.filePath,
		[]byte("doesn't matter, we don't execute it"),
		0755)
	c.Assert(err, jc.ErrorIsNil)

	cwd, err := os.Getwd()
	c.Assert(err, jc.ErrorIsNil)

	b.cwd = c.MkDir()
	err = os.Chdir(b.cwd)
	c.Assert(err, jc.ErrorIsNil)

	b.restore = func() {
		os.Setenv("PATH", path)
		os.Chdir(cwd)
	}
}

func (b *buildSuite) TearDownTest(c *gc.C) {
	b.restore()
	b.BaseSuite.TearDownTest(c)
}

func (b *buildSuite) TestFindExecutable(c *gc.C) {
	root := "/"
	if runtime.GOOS == "windows" {
		root = `C:\`
	}
	for _, test := range []struct {
		execFile   string
		expected   string
		errorMatch string
	}{{
		execFile: filepath.Join(root, "some", "absolute", "path"),
		expected: filepath.Join(root, "some", "absolute", "path"),
	}, {
		execFile: "./foo",
		expected: filepath.Join(b.cwd, "foo"),
	}, {
		execFile: "juju-test",
		expected: b.filePath,
	}, {
		execFile:   "non-existent-exec-file",
		errorMatch: `could not find "non-existent-exec-file" in the path`,
	}} {
		result, err := tools.FindExecutable(test.execFile)
		if test.errorMatch == "" {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(result, gc.Equals, test.expected)
		} else {
			c.Assert(err, gc.ErrorMatches, test.errorMatch)
			c.Assert(result, gc.Equals, "")
		}
	}
}

func (b *buildSuite) TestEmptyArchive(c *gc.C) {
	var buf bytes.Buffer
	dir := c.MkDir()
	err := tools.Archive(&buf, dir)
	c.Assert(err, jc.ErrorIsNil)

	gzr, err := gzip.NewReader(&buf)
	c.Assert(err, jc.ErrorIsNil)
	r := tar.NewReader(gzr)
	_, err = r.Next()
	c.Assert(err, gc.Equals, io.EOF)
}

func (b *buildSuite) TestArchiveAndSHA256(c *gc.C) {
	var buf bytes.Buffer
	dir := c.MkDir()
	sha256hash, err := tools.ArchiveAndSHA256(&buf, dir)
	c.Assert(err, jc.ErrorIsNil)

	h := sha256.New()
	h.Write(buf.Bytes())
	c.Assert(sha256hash, gc.Equals, fmt.Sprintf("%x", h.Sum(nil)))

	gzr, err := gzip.NewReader(&buf)
	c.Assert(err, jc.ErrorIsNil)
	r := tar.NewReader(gzr)
	_, err = r.Next()
	c.Assert(err, gc.Equals, io.EOF)
}

func (b *buildSuite) TestGetVersionFromJujud(c *gc.C) {
	ver := version.Binary{
		Number: version.Number{
			Major: 1,
			Minor: 2,
			Tag:   "beta",
			Patch: 1,
		},
		Release: "ubuntu",
		Arch:    "amd64",
	}

	argsCh := make(chan []string, 1)
	execCommand := b.GetExecCommand(exttest.PatchExecConfig{
		Stderr: "hey, here's some logging you should ignore",
		Stdout: ver.String(),
		Args:   argsCh,
	})

	b.PatchValue(&tools.ExecCommand, execCommand)

	v, err := tools.GetVersionFromJujud("foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(v, gc.Equals, ver)

	select {
	case args := <-argsCh:
		cmd := filepath.Join("foo", names.Jujud)
		c.Assert(args, gc.DeepEquals, []string{cmd, "version"})
	default:
		c.Fatalf("Failed to get args sent to executable.")
	}
}

func (b *buildSuite) TestGetVersionFromJujudWithParseError(c *gc.C) {
	argsCh := make(chan []string, 1)
	execCommand := b.GetExecCommand(exttest.PatchExecConfig{
		Stderr: "hey, here's some logging",
		Stdout: "oops, not a valid version",
		Args:   argsCh,
	})

	b.PatchValue(&tools.ExecCommand, execCommand)

	_, err := tools.GetVersionFromJujud("foo")
	c.Assert(err, gc.ErrorMatches, `invalid version "oops, not a valid version" printed by jujud`)

	select {
	case args := <-argsCh:
		cmd := filepath.Join("foo", names.Jujud)
		c.Assert(args, gc.DeepEquals, []string{cmd, "version"})
	default:
		c.Fatalf("Failed to get args sent to executable.")
	}
}

func (b *buildSuite) TestGetVersionFromJujudWithRunError(c *gc.C) {
	argsCh := make(chan []string, 1)
	execCommand := b.GetExecCommand(exttest.PatchExecConfig{
		Stderr:   "the stderr",
		Stdout:   "the stdout",
		ExitCode: 1,
		Args:     argsCh,
	})

	b.PatchValue(&tools.ExecCommand, execCommand)

	_, err := tools.GetVersionFromJujud("foo")

	cmd := filepath.Join("foo", names.Jujud)
	msg := fmt.Sprintf("cannot get version from %q: exit status 1; the stderr\nthe stdout\n", cmd)

	c.Assert(err.Error(), gc.Equals, msg)

	select {
	case args := <-argsCh:
		c.Assert(args, gc.DeepEquals, []string{cmd, "version"})
	default:
		c.Fatalf("Failed to get args sent to executable.")
	}
}

func (b *buildSuite) setUpFakeBinaries(c *gc.C, versionFile string) string {
	dir := c.MkDir()
	err := ioutil.WriteFile(filepath.Join(dir, "juju"), []byte("some data"), 0755)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(filepath.Join(dir, "jujuc"), []byte(fakeBinary), 0755)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(filepath.Join(dir, "jujud"), []byte(fakeBinary), 0755)
	c.Assert(err, jc.ErrorIsNil)
	if versionFile != "" {
		err = ioutil.WriteFile(filepath.Join(dir, "jujud-versions.yaml"), []byte(versionFile), 0755)
		c.Assert(err, jc.ErrorIsNil)
	}

	// Mock out args[0] so that copyExistingJujus can find our fake
	// binary. Tricky - we need to copy the test binary into the
	// directory so patching out exec can work.
	oldArg0 := os.Args[0]
	testBinary := filepath.Join(dir, "tst")
	os.Args[0] = testBinary
	err = os.Link(oldArg0, testBinary)
	if _, ok := err.(*os.LinkError); ok {
		// Soft link when cross device.
		err = os.Symlink(oldArg0, testBinary)
	}
	c.Assert(err, jc.ErrorIsNil)
	b.AddCleanup(func(c *gc.C) {
		os.Args[0] = oldArg0
	})
	return dir
}

func (b *buildSuite) TestBundleToolsIncludesVersionFile(c *gc.C) {
	dir := b.setUpFakeBinaries(c, fakeVersionFile)
	bundleFile, err := os.Create(filepath.Join(dir, "bundle"))
	c.Assert(err, jc.ErrorIsNil)

	resultVersion, official, sha256, err := tools.BundleTools(false, bundleFile, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Version should come from the version file.
	c.Assert(resultVersion.String(), gc.Equals, "1.2.3-ubuntu-arm64")
	c.Assert(official, jc.IsTrue)

	_, err = bundleFile.Seek(0, io.SeekStart)
	c.Assert(err, jc.ErrorIsNil)

	err = agenttools.UnpackTools(dir, &coretools.Tools{
		Version: resultVersion,
		SHA256:  sha256,
	}, bundleFile)
	c.Assert(err, jc.ErrorIsNil)

	unpackDir := filepath.Join(dir, "tools", "1.2.3-ubuntu-arm64")
	// downloaded-tools.txt is added by UnpackTools.
	c.Assert(listDir(c, unpackDir), gc.DeepEquals, []string{
		"downloaded-tools.txt", "jujuc", "jujud", "jujud-versions.yaml"})
}

func listDir(c *gc.C, dir string) []string {
	entries, err := ioutil.ReadDir(dir)
	c.Assert(err, jc.ErrorIsNil)
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func (b *buildSuite) TestBundleToolsMatchesBinaryUsingOsTypeArch(c *gc.C) {
	thisArch := arch.HostArch()
	thisHost := coreos.HostOSTypeName()
	dir := b.setUpFakeBinaries(c, fmt.Sprintf(osTypeArchMatchVersionFile, thisHost, thisArch))

	bundleFile, err := os.Create(filepath.Join(dir, "bundle"))
	c.Assert(err, jc.ErrorIsNil)

	resultVersion, official, _, err := tools.BundleTools(false, bundleFile, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resultVersion.String(), gc.Equals, fmt.Sprintf("1.2.3-%s-%s", thisHost, thisArch))
	c.Assert(official, jc.IsTrue)
}

func (b *buildSuite) TestJujudVersion(c *gc.C) {
	b.patchExecCommand(c)
	dir := b.setUpFakeBinaries(c, "")

	resultVersion, official, err := tools.JujudVersion(dir)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resultVersion.String(), gc.Equals, "1.2.3-ubuntu-amd64")
	c.Assert(official, jc.IsFalse)
}

func (b *buildSuite) TestBundleToolsWithNoVersionFile(c *gc.C) {
	b.patchExecCommand(c)
	dir := b.setUpFakeBinaries(c, "")
	bundleFile, err := os.Create(filepath.Join(dir, "bundle"))
	c.Assert(err, jc.ErrorIsNil)

	forceVersion := version.MustParse("1.2.3.1")
	resultVersion, official, sha, err := tools.BundleTools(false, bundleFile, &forceVersion)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resultVersion.String(), gc.Equals, "1.2.3-ubuntu-amd64")
	c.Assert(sha, gc.Not(gc.Equals), "")
	c.Assert(official, jc.IsFalse)
}

func (b *buildSuite) TestBundleToolsFailForOfficialBuildWithBuildAgent(c *gc.C) {
	b.patchExecCommand(c)
	dir := b.setUpFakeBinaries(c, "")
	bundleFile, err := os.Create(filepath.Join(dir, "bundle"))
	c.Assert(err, jc.ErrorIsNil)

	jujudVersion := func(dir string) (version.Binary, bool, error) {
		return version.Binary{}, true, nil
	}

	forceVersion := version.MustParse("1.2.3.1")
	_, official, _, err := tools.BundleToolsForTest(true, bundleFile, &forceVersion, jujudVersion)
	c.Assert(err, gc.ErrorMatches, `cannot build agent for official build`)
	c.Assert(official, jc.IsTrue)
}

func (b *buildSuite) TestBundleToolsWriteForceVersionFileForOfficial(c *gc.C) {
	b.patchExecCommand(c)
	dir := b.setUpFakeBinaries(c, "")
	bundleFile, err := os.Create(filepath.Join(dir, "bundle"))
	c.Assert(err, jc.ErrorIsNil)

	jujudVersion := func(dir string) (version.Binary, bool, error) {
		return version.Binary{}, true, nil
	}

	forceVersion := version.MustParse("1.2.3.1")
	_, official, _, err := tools.BundleToolsForTest(false, bundleFile, &forceVersion, jujudVersion)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(official, jc.IsTrue)

	bundleFile, err = os.Open(bundleFile.Name())
	c.Assert(err, jc.ErrorIsNil)
	gzr, err := gzip.NewReader(bundleFile)
	c.Assert(err, jc.ErrorIsNil)
	tarReader := tar.NewReader(gzr)

	timeout := time.After(testing.ShortWait)
	for {
		select {
		case <-timeout:
			c.Fatalf("ForceVersion File is not written as expected")
		default:
		}
		header, err := tarReader.Next()
		if err == io.EOF {
			c.Fatalf("ForceVersion File is not written as expected")
		}
		c.Assert(err, jc.ErrorIsNil)
		if header.Typeflag == tar.TypeReg && header.Name == "FORCE-VERSION" {
			forceVersionFile, err := ioutil.ReadAll(tarReader)
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(string(forceVersionFile), gc.Equals, `1.2.3.1`)
			break
		}
	}
}

func (b *buildSuite) patchExecCommand(c *gc.C) {
	// Patch so that getting the version from our fake binary in the
	// absence of a version file works.
	ver := version.Binary{
		Number: version.Number{
			Major: 1,
			Minor: 2,
			Patch: 3,
		},
		Release: "ubuntu",
		Arch:    "amd64",
	}
	execCommand := b.GetExecCommand(exttest.PatchExecConfig{
		Stdout: ver.String(),
		Args:   make(chan []string, 2),
	})
	b.PatchValue(&tools.ExecCommand, execCommand)
}

func (b *buildSuite) TestBundleToolsFindsVersionFileInFallbackLocation(c *gc.C) {
	b.patchExecCommand(c)

	// No version file next to the binary.
	dir := b.setUpFakeBinaries(c, "")
	// But one in the fallback location.
	err := ioutil.WriteFile(
		filepath.Join(tools.VersionFileFallbackDir, "jujud-versions.yaml"),
		[]byte(fakeVersionFile),
		0755)
	c.Assert(err, jc.ErrorIsNil)

	bundleFile, err := os.Create(filepath.Join(dir, "bundle"))
	c.Assert(err, jc.ErrorIsNil)

	resultVersion, official, sha256, err := tools.BundleTools(false, bundleFile, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Version should come from the version file.
	c.Assert(resultVersion.String(), gc.Equals, "1.2.3-ubuntu-arm64")
	c.Assert(official, jc.IsTrue)

	_, err = bundleFile.Seek(0, io.SeekStart)
	c.Assert(err, jc.ErrorIsNil)

	err = agenttools.UnpackTools(dir, &coretools.Tools{
		Version: resultVersion,
		SHA256:  sha256,
	}, bundleFile)
	c.Assert(err, jc.ErrorIsNil)

	unpackDir := filepath.Join(dir, "tools", "1.2.3-ubuntu-arm64")
	// downloaded-tools.txt is added by UnpackTools.
	c.Assert(listDir(c, unpackDir), gc.DeepEquals, []string{
		"downloaded-tools.txt", "jujuc", "jujud", "jujud-versions.yaml"})
}

func (b *buildSuite) TestBundleToolsUsesAdjacentVersionFirst(c *gc.C) {
	// If there are version files both beside the binary and in
	// /usr/lib/juju, use the one beside the binary.
	dir := b.setUpFakeBinaries(c, strings.Replace(fakeVersionFile, "1.2.3", "2.3.5", 1))
	err := ioutil.WriteFile(
		filepath.Join(tools.VersionFileFallbackDir, "jujud-versions.yaml"),
		[]byte(fakeVersionFile),
		0755)
	c.Assert(err, jc.ErrorIsNil)

	bundleFile, err := os.Create(filepath.Join(dir, "bundle"))
	c.Assert(err, jc.ErrorIsNil)

	resultVersion, official, _, err := tools.BundleTools(false, bundleFile, nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(resultVersion.String(), gc.Equals, "2.3.5-ubuntu-arm64")
	c.Assert(official, jc.IsTrue)
}

const (
	fakeBinary = "some binary content\n"
)

var (
	fakeVersionFile = `
versions:
  - version: 1.2.3-ubuntu-arm64
    sha256: b6813a18f82b16ae8d0cfb9e3063302688906e0c547db629a94dfb7f70198f00
  - version: 1.2.4-windows-amd64
    sha256: aaaa059f4cb8e83405fe6daabaa3ae62ead64ff841e0c26064c3e111c857e1fb
`[1:]

	osTypeArchMatchVersionFile = `
versions:
  - version: 1.2.3-ubuntu-arm64
    sha256: b6813a18f82b16ae8d0cfb9e3063302688906e0c547db629a94dfb7f70198f00
  - version: 1.2.3-%s-%s
    sha256: b6813a18f82b16ae8d0cfb9e3063302688906e0c547db629a94dfb7f70198f00
`[1:]
)
