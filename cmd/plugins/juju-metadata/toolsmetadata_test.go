// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/juju/cmd"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/environs/tools"
	toolstesting "github.com/juju/juju/environs/tools/testing"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/provider/dummy"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type ToolsMetadataSuite struct {
	coretesting.FakeJujuHomeSuite
	env              environs.Environ
	publicStorageDir string
}

var _ = gc.Suite(&ToolsMetadataSuite{})

func (s *ToolsMetadataSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.AddCleanup(func(*gc.C) {
		dummy.Reset()
		loggo.ResetLoggers()
	})
	env, err := environs.PrepareFromName(
		"erewhemos", envcmd.BootstrapContextNoVerify(coretesting.Context(c)), configstore.NewMem())
	c.Assert(err, jc.ErrorIsNil)
	s.env = env
	loggo.GetLogger("").SetLogLevel(loggo.INFO)

	// Switch the default tools location.
	s.publicStorageDir = c.MkDir()
	s.PatchValue(&tools.DefaultBaseURL, s.publicStorageDir)
}

var currentVersionStrings = []string{
	// only these ones will make it into the JSON files.
	version.Current.Number.String() + "-quantal-amd64",
	version.Current.Number.String() + "-quantal-armhf",
	version.Current.Number.String() + "-quantal-i386",
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
	expected := `Finding tools in .*
.*Fetching tools from dir "{{.ToolsDir}}" to generate hash: 1\.12\.0-precise-amd64
.*Fetching tools from dir "{{.ToolsDir}}" to generate hash: 1\.12\.0-precise-i386
.*Fetching tools from dir "{{.ToolsDir}}" to generate hash: 1\.12\.0-raring-amd64
.*Fetching tools from dir "{{.ToolsDir}}" to generate hash: 1\.12\.0-raring-i386
.*Fetching tools from dir "{{.ToolsDir}}" to generate hash: 1\.13\.0-precise-amd64
`
	f := `.*Fetching tools from dir "{{.ToolsDir}}" to generate hash: %s` + "\n"
	for _, v := range currentVersionStrings {
		expected += fmt.Sprintf(f, regexp.QuoteMeta(v))
	}
	return strings.TrimSpace(expected)
}

func makeExpectedOutput(templ, stream, toolsDir string) string {
	t := template.Must(template.New("").Parse(templ))

	var buf bytes.Buffer
	err := t.Execute(&buf, map[string]interface{}{"Stream": stream, "ToolsDir": toolsDir})
	if err != nil {
		panic(err)
	}
	return buf.String()
}

var expectedOutputDirectoryReleasedTemplate = expectedOutputCommon + `
.*Writing tools/streams/v1/index2\.json
.*Writing tools/streams/v1/index\.json
.*Writing tools/streams/v1/com\.ubuntu\.juju-{{.Stream}}-tools\.json
`

var expectedOutputDirectoryTemplate = expectedOutputCommon + `
.*Writing tools/streams/v1/index2\.json
.*Writing tools/streams/v1/com\.ubuntu\.juju-{{.Stream}}-tools\.json
`

var expectedOutputMirrorsTemplate = expectedOutputCommon + `
.*Writing tools/streams/v1/index2\.json
.*Writing tools/streams/v1/index\.json
.*Writing tools/streams/v1/com\.ubuntu\.juju-{{.Stream}}-tools\.json
.*Writing tools/streams/v1/mirrors\.json
`

var expectedOutputDirectoryLegacyReleased = "No stream specified, defaulting to released tools in the releases directory.\n" +
	makeExpectedOutput(expectedOutputDirectoryReleasedTemplate, "released", "releases")

var expectedOutputMirrorsReleased = makeExpectedOutput(expectedOutputMirrorsTemplate, "released", "released")

func (s *ToolsMetadataSuite) TestGenerateLegacyRelease(c *gc.C) {
	metadataDir := osenv.JujuHome() // default metadata dir
	toolstesting.MakeTools(c, metadataDir, "releases", versionStrings)
	ctx := coretesting.Context(c)
	code := cmd.Main(envcmd.Wrap(&ToolsMetadataCommand{}), ctx, nil)
	c.Assert(code, gc.Equals, 0)
	output := ctx.Stdout.(*bytes.Buffer).String()
	c.Assert(output, gc.Matches, expectedOutputDirectoryLegacyReleased)
	metadata := toolstesting.ParseMetadataFromDir(c, metadataDir, "released", false)
	c.Assert(metadata, gc.HasLen, len(versionStrings))
	obtainedVersionStrings := make([]string, len(versionStrings))
	for i, metadata := range metadata {
		s := fmt.Sprintf("%s-%s-%s", metadata.Version, metadata.Release, metadata.Arch)
		obtainedVersionStrings[i] = s
	}
	c.Assert(obtainedVersionStrings, gc.DeepEquals, versionStrings)
}

func (s *ToolsMetadataSuite) TestGenerateToDirectory(c *gc.C) {
	metadataDir := c.MkDir()
	toolstesting.MakeTools(c, metadataDir, "releases", versionStrings)
	ctx := coretesting.Context(c)
	code := cmd.Main(envcmd.Wrap(&ToolsMetadataCommand{}), ctx, []string{"-d", metadataDir})
	c.Assert(code, gc.Equals, 0)
	output := ctx.Stdout.(*bytes.Buffer).String()
	c.Assert(output, gc.Matches, expectedOutputDirectoryLegacyReleased)
	metadata := toolstesting.ParseMetadataFromDir(c, metadataDir, "released", false)
	c.Assert(metadata, gc.HasLen, len(versionStrings))
	obtainedVersionStrings := make([]string, len(versionStrings))
	for i, metadata := range metadata {
		s := fmt.Sprintf("%s-%s-%s", metadata.Version, metadata.Release, metadata.Arch)
		obtainedVersionStrings[i] = s
	}
	c.Assert(obtainedVersionStrings, gc.DeepEquals, versionStrings)
}

func (s *ToolsMetadataSuite) TestGenerateStream(c *gc.C) {
	metadataDir := c.MkDir()
	toolstesting.MakeTools(c, metadataDir, "proposed", versionStrings)
	ctx := coretesting.Context(c)
	code := cmd.Main(envcmd.Wrap(&ToolsMetadataCommand{}), ctx, []string{"-d", metadataDir, "--stream", "proposed"})
	c.Assert(code, gc.Equals, 0)
	output := ctx.Stdout.(*bytes.Buffer).String()
	c.Assert(output, gc.Matches, makeExpectedOutput(expectedOutputDirectoryTemplate, "proposed", "proposed"))
	metadata := toolstesting.ParseMetadataFromDir(c, metadataDir, "proposed", false)
	c.Assert(metadata, gc.HasLen, len(versionStrings))
	obtainedVersionStrings := make([]string, len(versionStrings))
	for i, metadata := range metadata {
		s := fmt.Sprintf("%s-%s-%s", metadata.Version, metadata.Release, metadata.Arch)
		obtainedVersionStrings[i] = s
	}
	c.Assert(obtainedVersionStrings, gc.DeepEquals, versionStrings)
}

func (s *ToolsMetadataSuite) TestGenerateMultipleStreams(c *gc.C) {
	metadataDir := c.MkDir()
	toolstesting.MakeTools(c, metadataDir, "proposed", versionStrings)
	toolstesting.MakeTools(c, metadataDir, "released", currentVersionStrings)

	ctx := coretesting.Context(c)
	code := cmd.Main(envcmd.Wrap(&ToolsMetadataCommand{}), ctx, []string{"-d", metadataDir, "--stream", "proposed"})
	c.Assert(code, gc.Equals, 0)
	code = cmd.Main(envcmd.Wrap(&ToolsMetadataCommand{}), ctx, []string{"-d", metadataDir, "--stream", "released"})
	c.Assert(code, gc.Equals, 0)

	metadata := toolstesting.ParseMetadataFromDir(c, metadataDir, "proposed", false)
	c.Assert(metadata, gc.HasLen, len(versionStrings))
	obtainedVersionStrings := make([]string, len(versionStrings))
	for i, metadata := range metadata {
		s := fmt.Sprintf("%s-%s-%s", metadata.Version, metadata.Release, metadata.Arch)
		obtainedVersionStrings[i] = s
	}
	c.Assert(obtainedVersionStrings, gc.DeepEquals, versionStrings)

	metadata = toolstesting.ParseMetadataFromDir(c, metadataDir, "released", false)
	c.Assert(metadata, gc.HasLen, len(currentVersionStrings))
	obtainedVersionStrings = make([]string, len(currentVersionStrings))
	for i, metadata := range metadata {
		s := fmt.Sprintf("%s-%s-%s", metadata.Version, metadata.Release, metadata.Arch)
		obtainedVersionStrings[i] = s
	}
	c.Assert(obtainedVersionStrings, gc.DeepEquals, currentVersionStrings)

	toolstesting.MakeTools(c, metadataDir, "released", versionStrings)
	metadata = toolstesting.ParseMetadataFromDir(c, metadataDir, "released", false)
	c.Assert(metadata, gc.HasLen, len(versionStrings))
	obtainedVersionStrings = make([]string, len(versionStrings))
	for i, metadata := range metadata {
		s := fmt.Sprintf("%s-%s-%s", metadata.Version, metadata.Release, metadata.Arch)
		obtainedVersionStrings[i] = s
	}
	c.Assert(obtainedVersionStrings, gc.DeepEquals, versionStrings)
}

func (s *ToolsMetadataSuite) TestGenerateDeleteExisting(c *gc.C) {
	metadataDir := c.MkDir()
	toolstesting.MakeTools(c, metadataDir, "proposed", versionStrings)
	toolstesting.MakeTools(c, metadataDir, "released", currentVersionStrings)

	ctx := coretesting.Context(c)
	code := cmd.Main(envcmd.Wrap(&ToolsMetadataCommand{}), ctx, []string{"-d", metadataDir, "--stream", "proposed"})
	c.Assert(code, gc.Equals, 0)
	code = cmd.Main(envcmd.Wrap(&ToolsMetadataCommand{}), ctx, []string{"-d", metadataDir, "--stream", "released"})
	c.Assert(code, gc.Equals, 0)

	// Remove existing proposed tarballs, and create some different ones.
	err := os.RemoveAll(filepath.Join(metadataDir, "tools", "proposed"))
	c.Assert(err, jc.ErrorIsNil)
	toolstesting.MakeTools(c, metadataDir, "proposed", currentVersionStrings)

	// Generate proposed metadata again, using --clean.
	code = cmd.Main(envcmd.Wrap(&ToolsMetadataCommand{}), ctx, []string{"-d", metadataDir, "--stream", "proposed", "--clean"})
	c.Assert(code, gc.Equals, 0)

	// Proposed metadata should just list the tarballs that were there, not the merged set.
	metadata := toolstesting.ParseMetadataFromDir(c, metadataDir, "proposed", false)
	c.Assert(metadata, gc.HasLen, len(currentVersionStrings))
	obtainedVersionStrings := make([]string, len(currentVersionStrings))
	for i, metadata := range metadata {
		s := fmt.Sprintf("%s-%s-%s", metadata.Version, metadata.Release, metadata.Arch)
		obtainedVersionStrings[i] = s
	}
	c.Assert(obtainedVersionStrings, gc.DeepEquals, currentVersionStrings)

	// Released metadata should be untouched.
	metadata = toolstesting.ParseMetadataFromDir(c, metadataDir, "released", false)
	c.Assert(metadata, gc.HasLen, len(currentVersionStrings))
	obtainedVersionStrings = make([]string, len(currentVersionStrings))
	for i, metadata := range metadata {
		s := fmt.Sprintf("%s-%s-%s", metadata.Version, metadata.Release, metadata.Arch)
		obtainedVersionStrings[i] = s
	}
	c.Assert(obtainedVersionStrings, gc.DeepEquals, currentVersionStrings)
}

func (s *ToolsMetadataSuite) TestGenerateWithPublicFallback(c *gc.C) {
	// Write tools and metadata to the public tools location.
	toolstesting.MakeToolsWithCheckSum(c, s.publicStorageDir, "released", versionStrings)

	// Run the command with no local metadata.
	ctx := coretesting.Context(c)
	metadataDir := c.MkDir()
	code := cmd.Main(envcmd.Wrap(&ToolsMetadataCommand{}), ctx, []string{"-d", metadataDir, "--stream", "released"})
	c.Assert(code, gc.Equals, 0)
	metadata := toolstesting.ParseMetadataFromDir(c, metadataDir, "released", false)
	c.Assert(metadata, gc.HasLen, len(versionStrings))
	obtainedVersionStrings := make([]string, len(versionStrings))
	for i, metadata := range metadata {
		s := fmt.Sprintf("%s-%s-%s", metadata.Version, metadata.Release, metadata.Arch)
		obtainedVersionStrings[i] = s
	}
	c.Assert(obtainedVersionStrings, gc.DeepEquals, versionStrings)
}

func (s *ToolsMetadataSuite) TestGenerateWithMirrors(c *gc.C) {
	metadataDir := c.MkDir()
	toolstesting.MakeTools(c, metadataDir, "released", versionStrings)
	ctx := coretesting.Context(c)
	code := cmd.Main(envcmd.Wrap(&ToolsMetadataCommand{}), ctx, []string{"--public", "-d", metadataDir, "--stream", "released"})
	c.Assert(code, gc.Equals, 0)
	output := ctx.Stdout.(*bytes.Buffer).String()
	c.Assert(output, gc.Matches, expectedOutputMirrorsReleased)
	metadata := toolstesting.ParseMetadataFromDir(c, metadataDir, "released", true)
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
	code := cmd.Main(envcmd.Wrap(&ToolsMetadataCommand{}), ctx, nil)
	c.Assert(code, gc.Equals, 1)
	stdout := ctx.Stdout.(*bytes.Buffer).String()
	c.Assert(stdout, gc.Matches, ".*\nFinding tools in .*\n")
	stderr := ctx.Stderr.(*bytes.Buffer).String()
	c.Assert(stderr, gc.Matches, "error: no tools available\n")
}

func (s *ToolsMetadataSuite) TestPatchLevels(c *gc.C) {
	currentVersion := version.Current.Number
	currentVersion.Build = 0
	versionStrings := []string{
		currentVersion.String() + "-precise-amd64",
		currentVersion.String() + ".1-precise-amd64",
	}
	metadataDir := osenv.JujuHome() // default metadata dir
	toolstesting.MakeTools(c, metadataDir, "released", versionStrings)
	ctx := coretesting.Context(c)
	code := cmd.Main(envcmd.Wrap(&ToolsMetadataCommand{}), ctx, []string{"--stream", "released"})
	c.Assert(code, gc.Equals, 0)
	output := ctx.Stdout.(*bytes.Buffer).String()
	expectedOutput := fmt.Sprintf(`
Finding tools in .*
.*Fetching tools from dir "released" to generate hash: %s
.*Fetching tools from dir "released" to generate hash: %s
.*Writing tools/streams/v1/index2\.json
.*Writing tools/streams/v1/index\.json
.*Writing tools/streams/v1/com\.ubuntu\.juju-released-tools\.json
`[1:], regexp.QuoteMeta(versionStrings[0]), regexp.QuoteMeta(versionStrings[1]))
	c.Assert(output, gc.Matches, expectedOutput)
	metadata := toolstesting.ParseMetadataFromDir(c, metadataDir, "released", false)
	c.Assert(metadata, gc.HasLen, 2)

	filename := fmt.Sprintf("juju-%s-precise-amd64.tgz", currentVersion)
	size, sha256 := toolstesting.SHA256sum(c, filepath.Join(metadataDir, "tools", "released", filename))
	c.Assert(metadata[0], gc.DeepEquals, &tools.ToolsMetadata{
		Release:  "precise",
		Version:  currentVersion.String(),
		Arch:     "amd64",
		Size:     size,
		Path:     "released/" + filename,
		FileType: "tar.gz",
		SHA256:   sha256,
	})

	filename = fmt.Sprintf("juju-%s.1-precise-amd64.tgz", currentVersion)
	size, sha256 = toolstesting.SHA256sum(c, filepath.Join(metadataDir, "tools", "released", filename))
	c.Assert(metadata[1], gc.DeepEquals, &tools.ToolsMetadata{
		Release:  "precise",
		Version:  currentVersion.String() + ".1",
		Arch:     "amd64",
		Size:     size,
		Path:     "released/" + filename,
		FileType: "tar.gz",
		SHA256:   sha256,
	})
}
