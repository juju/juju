// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	gc "launchpad.net/gocheck"
	"launchpad.net/loggo"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/environs/tools"
	ttesting "launchpad.net/juju-core/environs/tools/testing"
	_ "launchpad.net/juju-core/provider/dummy"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
)

type ToolsMetadataSuite struct {
	home *coretesting.FakeHome
	env  environs.Environ
}

var _ = gc.Suite(&ToolsMetadataSuite{})

func (s *ToolsMetadataSuite) SetUpTest(c *gc.C) {
	s.home = coretesting.MakeSampleHome(c)
	env, err := environs.PrepareFromName("erewhemos")
	c.Assert(err, gc.IsNil)
	s.env = env
	envtesting.RemoveAllTools(c, s.env)
	loggo.GetLogger("").SetLogLevel(loggo.INFO)
}

func (s *ToolsMetadataSuite) TearDownTest(c *gc.C) {
	loggo.ResetLoggers()
	s.home.Restore()
}

var currentVersionStrings = []string{
	// only these ones will make it into the JSON files.
	version.CurrentNumber().String() + "-quantal-amd64",
	version.CurrentNumber().String() + "-quantal-arm",
	version.CurrentNumber().String() + "-quantal-i386",
}

var versionStrings = append([]string{
	"1.12.0-precise-amd64",
	"1.12.0-precise-i386",
	"1.12.0-raring-amd64",
	"1.12.0-raring-i386",
	"1.13.0-precise-amd64",
}, currentVersionStrings...)

var expectedOutputCommon = makeExpectedOutputCommon()

func makeExpectedOutputCommon() string {
	expected := `Finding tools\.\.\.
.*Fetching tools to generate hash: .*/tools/.*juju-1\.12\.0-precise-amd64\.tgz
.*Fetching tools to generate hash: .*/tools/.*juju-1\.12\.0-precise-i386\.tgz
.*Fetching tools to generate hash: .*/tools/.*juju-1\.12\.0-raring-amd64\.tgz
.*Fetching tools to generate hash: .*/tools/.*juju-1\.12\.0-raring-i386\.tgz
.*Fetching tools to generate hash: .*/tools/.*juju-1\.13\.0-precise-amd64\.tgz
`
	f := ".*Fetching tools to generate hash: .*/tools/.*juju-%s\\.tgz\n"
	for _, v := range currentVersionStrings {
		expected += fmt.Sprintf(f, regexp.QuoteMeta(v))
	}
	return strings.TrimSpace(expected)
}

var expectedOutputDirectory = expectedOutputCommon + `
.*Writing tools/streams/v1/index\.json
.*Writing tools/streams/v1/com\.ubuntu\.juju:released:tools\.json
`

func (s *ToolsMetadataSuite) assertGenerateDefaultDirectory(c *gc.C, subdir string) {
	metadataDir := config.JujuHome() // default metadata dir
	ttesting.MakeTools(c, metadataDir, subdir, versionStrings)
	ctx := coretesting.Context(c)
	oldWriter, err := loggo.ReplaceDefaultWriter(loggo.NewSimpleWriter(ctx.Stdout, &loggo.DefaultFormatter{}))
	c.Assert(err, gc.IsNil)
	defer loggo.ReplaceDefaultWriter(oldWriter)

	code := cmd.Main(&ToolsMetadataCommand{noS3: true}, ctx, nil)
	c.Assert(code, gc.Equals, 0)
	output := ctx.Stdout.(*bytes.Buffer).String()
	c.Assert(output, gc.Matches, expectedOutputDirectory)
	metadata := ttesting.ParseMetadata(c, metadataDir)
	c.Assert(metadata, gc.HasLen, len(versionStrings))
	obtainedVersionStrings := make([]string, len(versionStrings))
	for i, metadata := range metadata {
		s := fmt.Sprintf("%s-%s-%s", metadata.Version, metadata.Release, metadata.Arch)
		obtainedVersionStrings[i] = s
	}
	c.Assert(obtainedVersionStrings, gc.DeepEquals, versionStrings)
}

func (s *ToolsMetadataSuite) TestGenerateDefaultDirectoryLegacyToolsLocation(c *gc.C) {
	s.assertGenerateDefaultDirectory(c, "")
}

func (s *ToolsMetadataSuite) TestGenerateDefaultDirectory(c *gc.C) {
	s.assertGenerateDefaultDirectory(c, "releases")
}

func (s *ToolsMetadataSuite) TestGenerateDirectory(c *gc.C) {
	metadataDir := c.MkDir()
	ttesting.MakeTools(c, metadataDir, "releases", versionStrings)
	ctx := coretesting.Context(c)
	oldWriter, err := loggo.ReplaceDefaultWriter(loggo.NewSimpleWriter(ctx.Stdout, &loggo.DefaultFormatter{}))
	c.Assert(err, gc.IsNil)
	defer loggo.ReplaceDefaultWriter(oldWriter)

	code := cmd.Main(&ToolsMetadataCommand{noS3: true}, ctx, []string{"-d", metadataDir})
	c.Assert(code, gc.Equals, 0)
	output := ctx.Stdout.(*bytes.Buffer).String()
	c.Assert(output, gc.Matches, expectedOutputDirectory)
	metadata := ttesting.ParseMetadata(c, metadataDir)
	c.Assert(metadata, gc.HasLen, len(versionStrings))
	obtainedVersionStrings := make([]string, len(versionStrings))
	for i, metadata := range metadata {
		s := fmt.Sprintf("%s-%s-%s", metadata.Version, metadata.Release, metadata.Arch)
		obtainedVersionStrings[i] = s
	}
	c.Assert(obtainedVersionStrings, gc.DeepEquals, versionStrings)
}

func (s *ToolsMetadataSuite) TestNoTools(c *gc.C) {
	ctx := coretesting.Context(c)
	code := cmd.Main(&ToolsMetadataCommand{noS3: true}, ctx, nil)
	c.Assert(code, gc.Equals, 1)
	stdout := ctx.Stdout.(*bytes.Buffer).String()
	c.Assert(stdout, gc.Matches, "Finding tools\\.\\.\\.\n")
	stderr := ctx.Stderr.(*bytes.Buffer).String()
	c.Assert(stderr, gc.Matches, "error: no tools available\n")
}

func (s *ToolsMetadataSuite) TestPatchLevels(c *gc.C) {
	currentVersion := version.CurrentNumber()
	currentVersion.Build = 0
	versionStrings := []string{
		currentVersion.String() + "-precise-amd64",
		currentVersion.String() + ".1-precise-amd64",
	}
	metadataDir := config.JujuHome() // default metadata dir
	ttesting.MakeTools(c, metadataDir, "releases", versionStrings)
	ctx := coretesting.Context(c)
	oldWriter, err := loggo.ReplaceDefaultWriter(loggo.NewSimpleWriter(ctx.Stdout, &loggo.DefaultFormatter{}))
	c.Assert(err, gc.IsNil)
	defer loggo.ReplaceDefaultWriter(oldWriter)

	code := cmd.Main(&ToolsMetadataCommand{noS3: true}, ctx, nil)
	c.Assert(code, gc.Equals, 0)
	output := ctx.Stdout.(*bytes.Buffer).String()
	expectedOutput := fmt.Sprintf(`
Finding tools\.\.\.
.*Fetching tools to generate hash: .*/tools/releases/juju-%s\.tgz
.*Fetching tools to generate hash: .*/tools/releases/juju-%s\.tgz
.*Writing tools/streams/v1/index\.json
.*Writing tools/streams/v1/com\.ubuntu\.juju:released:tools\.json
`[1:], regexp.QuoteMeta(versionStrings[0]), regexp.QuoteMeta(versionStrings[1]))
	c.Assert(output, gc.Matches, expectedOutput)
	metadata := ttesting.ParseMetadata(c, metadataDir)
	c.Assert(metadata, gc.HasLen, 2)

	filename := fmt.Sprintf("juju-%s-precise-amd64.tgz", currentVersion)
	size, sha256 := ttesting.SHA256sum(c, filepath.Join(metadataDir, "tools", "releases", filename))
	c.Assert(metadata[0], gc.DeepEquals, &tools.ToolsMetadata{
		Release:  "precise",
		Version:  currentVersion.String(),
		Arch:     "amd64",
		Size:     size,
		Path:     "releases/" + filename,
		FileType: "tar.gz",
		SHA256:   sha256,
	})

	filename = fmt.Sprintf("juju-%s.1-precise-amd64.tgz", currentVersion)
	size, sha256 = ttesting.SHA256sum(c, filepath.Join(metadataDir, "tools", "releases", filename))
	c.Assert(metadata[1], gc.DeepEquals, &tools.ToolsMetadata{
		Release:  "precise",
		Version:  currentVersion.String() + ".1",
		Arch:     "amd64",
		Size:     size,
		Path:     "releases/" + filename,
		FileType: "tar.gz",
		SHA256:   sha256,
	})
}
