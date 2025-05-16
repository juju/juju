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
	"os"
	"path/filepath"
	"strings"
	stdtesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/core/arch"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/names"
)

type buildSuite struct {
	testing.BaseSuite
	restore  func()
	cwd      string
	filePath string
}

func TestBuildSuite(t *stdtesting.T) { tc.Run(t, &buildSuite{}) }
func (b *buildSuite) SetUpTest(c *tc.C) {
	b.BaseSuite.SetUpTest(c)
	dir1 := c.MkDir()
	dir2 := c.MkDir()

	// Ensure we don't look in the real /usr/lib/juju for jujud-versions.yaml.
	b.PatchValue(&tools.VersionFileFallbackDir, c.MkDir())

	c.Log(dir1)
	c.Log(dir2)

	path := os.Getenv("PATH")
	os.Setenv("PATH", strings.Join([]string{dir1, dir2, path}, string(filepath.ListSeparator)))

	// Make an executable file called "juju-test" in dir2.
	b.filePath = filepath.Join(dir2, "juju-test")
	err := os.WriteFile(
		b.filePath,
		[]byte("doesn't matter, we don't execute it"),
		0755)
	c.Assert(err, tc.ErrorIsNil)

	cwd, err := os.Getwd()
	c.Assert(err, tc.ErrorIsNil)

	b.cwd = c.MkDir()
	err = os.Chdir(b.cwd)
	c.Assert(err, tc.ErrorIsNil)

	b.restore = func() {
		os.Setenv("PATH", path)
		os.Chdir(cwd)
	}
}

func (b *buildSuite) TearDownTest(c *tc.C) {
	b.restore()
	b.BaseSuite.TearDownTest(c)
}

func (b *buildSuite) TestFindExecutable(c *tc.C) {
	for _, test := range []struct {
		execFile   string
		expected   string
		errorMatch string
	}{{
		execFile: filepath.Join("/", "some", "absolute", "path"),
		expected: filepath.Join("/", "some", "absolute", "path"),
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
		result, err := tools.FindExecutable(c.Context(), test.execFile)
		if test.errorMatch == "" {
			c.Assert(err, tc.ErrorIsNil)
			c.Assert(result, tc.Equals, test.expected)
		} else {
			c.Assert(err, tc.ErrorMatches, test.errorMatch)
			c.Assert(result, tc.Equals, "")
		}
	}
}

func (b *buildSuite) TestEmptyArchive(c *tc.C) {
	var buf bytes.Buffer
	dir := c.MkDir()
	err := tools.Archive(&buf, dir)
	c.Assert(err, tc.ErrorIsNil)

	gzr, err := gzip.NewReader(&buf)
	c.Assert(err, tc.ErrorIsNil)
	r := tar.NewReader(gzr)
	_, err = r.Next()
	c.Assert(err, tc.Equals, io.EOF)
}

func (b *buildSuite) TestArchiveAndSHA256(c *tc.C) {
	var buf bytes.Buffer
	dir := c.MkDir()
	sha256hash, err := tools.ArchiveAndSHA256(&buf, dir)
	c.Assert(err, tc.ErrorIsNil)

	h := sha256.New()
	h.Write(buf.Bytes())
	c.Assert(sha256hash, tc.Equals, fmt.Sprintf("%x", h.Sum(nil)))

	gzr, err := gzip.NewReader(&buf)
	c.Assert(err, tc.ErrorIsNil)
	r := tar.NewReader(gzr)
	_, err = r.Next()
	c.Assert(err, tc.Equals, io.EOF)
}

func (b *buildSuite) TestGetVersionFromJujud(c *tc.C) {
	ver := semversion.Binary{
		Number: semversion.Number{
			Major: 1,
			Minor: 2,
			Tag:   "beta",
			Patch: 1,
		},
		Release: "ubuntu",
		Arch:    "amd64",
	}

	argsCh := make(chan []string, 1)
	execCommand := testhelpers.ExecCommand(testhelpers.PatchExecConfig{
		Stderr: "hey, here's some logging you should ignore",
		Stdout: ver.String(),
		Args:   argsCh,
	})

	b.PatchValue(&tools.ExecCommand, execCommand)

	dir := c.MkDir()
	cmd := filepath.Join(dir, names.Jujud)
	err := os.WriteFile(cmd, []byte{}, 0644)
	c.Assert(err, tc.ErrorIsNil)
	v, err := tools.GetVersionFromJujud(dir)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(v, tc.Equals, ver)

	select {
	case args := <-argsCh:
		c.Assert(args, tc.DeepEquals, []string{cmd, "version"})
	default:
		c.Fatalf("Failed to get args sent to executable.")
	}
}

func (b *buildSuite) TestGetVersionFromJujudWithParseError(c *tc.C) {
	argsCh := make(chan []string, 1)
	execCommand := testhelpers.ExecCommand(testhelpers.PatchExecConfig{
		Stderr: "hey, here's some logging",
		Stdout: "oops, not a valid version",
		Args:   argsCh,
	})

	b.PatchValue(&tools.ExecCommand, execCommand)

	dir := c.MkDir()
	cmd := filepath.Join(dir, names.Jujud)
	err := os.WriteFile(cmd, []byte{}, 0644)
	c.Assert(err, tc.ErrorIsNil)
	_, err = tools.GetVersionFromJujud(dir)
	c.Assert(err, tc.ErrorMatches, `invalid version "oops, not a valid version" printed by jujud`)

	select {
	case args := <-argsCh:
		c.Assert(args, tc.DeepEquals, []string{cmd, "version"})
	default:
		c.Fatalf("Failed to get args sent to executable.")
	}
}

func (b *buildSuite) TestGetVersionFromJujudWithRunError(c *tc.C) {
	argsCh := make(chan []string, 1)
	execCommand := testhelpers.ExecCommand(testhelpers.PatchExecConfig{
		Stderr:   "the stderr",
		Stdout:   "the stdout",
		ExitCode: 1,
		Args:     argsCh,
	})

	b.PatchValue(&tools.ExecCommand, execCommand)

	dir := c.MkDir()
	cmd := filepath.Join(dir, names.Jujud)
	err := os.WriteFile(cmd, []byte{}, 0644)
	c.Assert(err, tc.ErrorIsNil)
	_, err = tools.GetVersionFromJujud(dir)

	msg := fmt.Sprintf("cannot get version from %q: exit status 1; the stderr\nthe stdout\n", cmd)

	c.Assert(err.Error(), tc.Equals, msg)

	select {
	case args := <-argsCh:
		c.Assert(args, tc.DeepEquals, []string{cmd, "version"})
	default:
		c.Fatalf("Failed to get args sent to executable.")
	}
}

func (b *buildSuite) TestGetVersionFromJujudNoJujud(c *tc.C) {
	execCommand := testhelpers.ExecCommand(testhelpers.PatchExecConfig{
		ExitCode: 1,
	})
	b.PatchValue(&tools.ExecCommand, execCommand)

	_, err := tools.GetVersionFromJujud("foo")
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (b *buildSuite) setUpFakeBinaries(c *tc.C, versionFile string) string {
	dir := c.MkDir()
	err := os.WriteFile(filepath.Join(dir, "juju"), []byte("some data"), 0755)
	c.Assert(err, tc.ErrorIsNil)
	err = os.WriteFile(filepath.Join(dir, "jujuc"), []byte(fakeBinary), 0755)
	c.Assert(err, tc.ErrorIsNil)
	err = os.WriteFile(filepath.Join(dir, "jujud"), []byte(fakeBinary), 0755)
	c.Assert(err, tc.ErrorIsNil)
	if versionFile != "" {
		err = os.WriteFile(filepath.Join(dir, "jujud-versions.yaml"), []byte(versionFile), 0755)
		c.Assert(err, tc.ErrorIsNil)
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
	c.Assert(err, tc.ErrorIsNil)
	b.AddCleanup(func(c *tc.C) {
		os.Args[0] = oldArg0
	})
	return dir
}

func (b *buildSuite) TestBundleToolsMatchesBinaryUsingOsTypeArch(c *tc.C) {
	thisArch := arch.HostArch()
	thisHost := coreos.HostOSTypeName()
	b.patchExecCommand(c, thisHost, thisArch)
	dir := b.setUpFakeBinaries(c, "")

	bundleFile, err := os.Create(filepath.Join(dir, "bundle"))
	c.Assert(err, tc.ErrorIsNil)

	resultVersion, forceVersion, official, _, err := tools.BundleTools(false, bundleFile,
		func(localBinaryVersion semversion.Number) semversion.Number { return semversion.MustParse("1.2.3.1") },
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(resultVersion.String(), tc.Equals, fmt.Sprintf("1.2.3-%s-%s", thisHost, thisArch))
	c.Assert(forceVersion, tc.Equals, semversion.MustParse("1.2.3.1"))
	c.Assert(official, tc.IsFalse)
}

func (b *buildSuite) TestJujudVersion(c *tc.C) {
	b.patchExecCommand(c, "", "")
	dir := b.setUpFakeBinaries(c, "")

	resultVersion, official, err := tools.JujudVersion(dir)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(resultVersion.String(), tc.Equals, "1.2.3-ubuntu-amd64")
	c.Assert(official, tc.IsFalse)
}

func (b *buildSuite) TestBundleToolsWithNoVersionFile(c *tc.C) {
	b.patchExecCommand(c, "", "")
	dir := b.setUpFakeBinaries(c, "")
	bundleFile, err := os.Create(filepath.Join(dir, "bundle"))
	c.Assert(err, tc.ErrorIsNil)

	resultVersion, forceVersion, official, sha, err := tools.BundleTools(false, bundleFile,
		func(localBinaryVersion semversion.Number) semversion.Number { return semversion.MustParse("1.2.3.1") },
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(resultVersion.String(), tc.Equals, "1.2.3-ubuntu-amd64")
	c.Assert(forceVersion, tc.Equals, semversion.MustParse("1.2.3.1"))
	c.Assert(sha, tc.Not(tc.Equals), "")
	c.Assert(official, tc.IsFalse)
}

func (b *buildSuite) TestBundleToolsFailForOfficialBuildWithBuildAgent(c *tc.C) {
	b.patchExecCommand(c, "", "")
	dir := b.setUpFakeBinaries(c, "")
	bundleFile, err := os.Create(filepath.Join(dir, "bundle"))
	c.Assert(err, tc.ErrorIsNil)

	jujudVersion := func(dir string) (semversion.Binary, bool, error) {
		return semversion.Binary{}, true, nil
	}

	_, _, official, _, err := tools.BundleToolsForTest(
		c.Context(),
		true, bundleFile,
		func(localBinaryVersion semversion.Number) semversion.Number { return semversion.MustParse("1.2.3.1") },
		jujudVersion)
	c.Assert(err, tc.ErrorMatches, `cannot build agent for official build`)
	c.Assert(official, tc.IsTrue)
}

func (b *buildSuite) TestBundleToolsWriteForceVersionFileForOfficial(c *tc.C) {
	b.patchExecCommand(c, "", "")
	dir := b.setUpFakeBinaries(c, "")
	bundleFile, err := os.Create(filepath.Join(dir, "bundle"))
	c.Assert(err, tc.ErrorIsNil)

	jujudVersion := func(dir string) (semversion.Binary, bool, error) {
		return semversion.Binary{}, true, nil
	}

	_, forceVersion, official, _, err := tools.BundleToolsForTest(
		c.Context(),
		false, bundleFile,
		func(localBinaryVersion semversion.Number) semversion.Number { return semversion.MustParse("1.2.3.1") },
		jujudVersion)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(forceVersion, tc.Equals, semversion.MustParse("1.2.3.1"))
	c.Assert(official, tc.IsTrue)

	bundleFile, err = os.Open(bundleFile.Name())
	c.Assert(err, tc.ErrorIsNil)
	gzr, err := gzip.NewReader(bundleFile)
	c.Assert(err, tc.ErrorIsNil)
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
		c.Assert(err, tc.ErrorIsNil)
		if header.Typeflag == tar.TypeReg && header.Name == "FORCE-VERSION" {
			forceVersionFile, err := io.ReadAll(tarReader)
			c.Assert(err, tc.ErrorIsNil)
			c.Assert(string(forceVersionFile), tc.Equals, `1.2.3.1`)
			break
		}
	}
}

func (b *buildSuite) patchExecCommand(c *tc.C, release, arch string) {
	// Patch so that getting the version from our fake binary in the
	// absence of a version file works.
	if release == "" {
		release = "ubuntu"
	}
	if arch == "" {
		arch = "amd64"
	}
	ver := semversion.Binary{
		Number: semversion.Number{
			Major: 1,
			Minor: 2,
			Patch: 3,
		},
		Release: release,
		Arch:    arch,
	}
	execCommand := testhelpers.ExecCommand(testhelpers.PatchExecConfig{
		Stdout: ver.String(),
		Args:   make(chan []string, 2),
	})
	b.PatchValue(&tools.ExecCommand, execCommand)
}

const (
	fakeBinary = "some binary content\n"
)
