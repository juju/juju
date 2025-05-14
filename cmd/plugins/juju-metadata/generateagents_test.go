// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"text/template"

	"github.com/juju/loggo/v2"
	"github.com/juju/tc"

	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	environscmd "github.com/juju/juju/environs/cmd"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/simplestreams"
	sstestings "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/environs/tools"
	toolstesting "github.com/juju/juju/environs/tools/testing"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	_ "github.com/juju/juju/internal/provider/dummy"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/keys"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
)

type GenerateAgentsSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	env              environs.Environ
	publicStorageDir string
}

var _ = tc.Suite(&GenerateAgentsSuite{})

func (s *GenerateAgentsSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	cfg, err := config.New(config.UseDefaults, map[string]interface{}{
		"name":            "erewhemos",
		"type":            "dummy",
		"uuid":            coretesting.ModelTag.Id(),
		"controller-uuid": coretesting.ControllerTag.Id(),
		"conroller":       true,
	})
	c.Assert(err, tc.ErrorIsNil)
	e, err := bootstrap.PrepareController(
		false,
		environscmd.BootstrapContextNoVerify(c.Context(), cmdtesting.Context(c)),
		jujuclient.NewMemStore(),
		bootstrap.PrepareParams{
			ControllerConfig: coretesting.FakeControllerConfig(),
			ControllerName:   cfg.Name(),
			ModelConfig:      cfg.AllAttrs(),
			Cloud:            coretesting.FakeCloudSpec(),
			AdminSecret:      "admin-secret",
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	s.env = e.(environs.Environ)
	loggo.GetLogger("").SetLogLevel(loggo.INFO)

	// Switch the default tools location.
	s.publicStorageDir = c.MkDir()
	s.PatchValue(&tools.DefaultBaseURL, s.publicStorageDir)
}

var currentVersionStrings = []string{
	// only these ones will make it into the JSON files.
	jujuversion.Current.String() + "-ubuntu-amd64",
	jujuversion.Current.String() + "-ubuntu-arm64",
}

var versionStrings = append([]string{
	fmt.Sprintf("%d.12.0-ubuntu-amd64", jujuversion.Current.Major),
	fmt.Sprintf("%d.12.0-ubuntu-arm64", jujuversion.Current.Major),
	fmt.Sprintf("%d.13.0-ubuntu-amd64", jujuversion.Current.Major+1),
}, currentVersionStrings...)

var expectedOutputCommon = makeExpectedOutputCommon()

func makeExpectedOutputCommon() string {
	expected := "Finding agent binaries in .*\n"
	f := `.*Fetching agent binaries from dir "{{.ToolsDir}}" to generate hash: %s` + "\n"

	// Sort the global versionStrings
	sort.Strings(versionStrings)
	for _, v := range versionStrings {
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

var expectedOutputDirectoryTemplate = expectedOutputCommon + `
.*Writing tools/streams/v1/index2\.json
.*Writing tools/streams/v1/com\.ubuntu\.juju-{{.Stream}}-agents\.json
`

func newGenerateAgentsCommandForTests() cmd.Command {
	return &generateAgentsCommand{}
}

func (s *GenerateAgentsSuite) TestGenerateToDirectory(c *tc.C) {
	metadataDir := c.MkDir()
	toolstesting.MakeTools(c, metadataDir, "released", versionStrings)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(newGenerateAgentsCommandForTests(), ctx, []string{"-d", metadataDir})
	c.Check(code, tc.Equals, 0)
	output := ctx.Stdout.(*bytes.Buffer).String()

	outputDirReleasedTmpl := expectedOutputCommon + `
.*Writing tools/streams/v1/index2\.json
.*Writing tools/streams/v1/index\.json
.*Writing tools/streams/v1/com\.ubuntu\.juju-{{.Stream}}-agents\.json
`
	expectedOutput := makeExpectedOutput(outputDirReleasedTmpl, "released", "released")
	c.Check(output, tc.Matches, expectedOutput)
	metadata := toolstesting.ParseMetadataFromDir(c, metadataDir, "released", false)
	c.Check(metadata, tc.HasLen, len(versionStrings))
	obtainedVersionStrings := make([]string, len(versionStrings))
	for i, metadata := range metadata {
		s := fmt.Sprintf("%s-%s-%s", metadata.Version, metadata.Release, metadata.Arch)
		obtainedVersionStrings[i] = s
	}
	c.Check(obtainedVersionStrings, tc.DeepEquals, versionStrings)
}

func (s *GenerateAgentsSuite) TestGenerateStream(c *tc.C) {
	metadataDir := c.MkDir()
	toolstesting.MakeTools(c, metadataDir, "proposed", versionStrings)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(newGenerateAgentsCommandForTests(), ctx, []string{"-d", metadataDir, "--stream", "proposed"})
	c.Assert(code, tc.Equals, 0)
	output := ctx.Stdout.(*bytes.Buffer).String()
	c.Assert(output, tc.Matches, makeExpectedOutput(expectedOutputDirectoryTemplate, "proposed", "proposed"))
	metadata := toolstesting.ParseMetadataFromDir(c, metadataDir, "proposed", false)
	c.Assert(metadata, tc.HasLen, len(versionStrings))
	obtainedVersionStrings := make([]string, len(versionStrings))
	for i, metadata := range metadata {
		s := fmt.Sprintf("%s-%s-%s", metadata.Version, metadata.Release, metadata.Arch)
		obtainedVersionStrings[i] = s
	}
	c.Assert(obtainedVersionStrings, tc.DeepEquals, versionStrings)
}

func (s *GenerateAgentsSuite) TestGenerateMultipleStreams(c *tc.C) {
	metadataDir := c.MkDir()
	toolstesting.MakeTools(c, metadataDir, "proposed", versionStrings)
	toolstesting.MakeTools(c, metadataDir, "released", currentVersionStrings)

	ctx := cmdtesting.Context(c)
	code := cmd.Main(newGenerateAgentsCommandForTests(), ctx, []string{"-d", metadataDir, "--stream", "proposed"})
	c.Assert(code, tc.Equals, 0)
	code = cmd.Main(newGenerateAgentsCommandForTests(), ctx, []string{"-d", metadataDir, "--stream", "released"})
	c.Assert(code, tc.Equals, 0)

	metadata := toolstesting.ParseMetadataFromDir(c, metadataDir, "proposed", false)
	c.Assert(metadata, tc.HasLen, len(versionStrings))
	obtainedVersionStrings := make([]string, len(versionStrings))
	for i, metadata := range metadata {
		s := fmt.Sprintf("%s-%s-%s", metadata.Version, metadata.Release, metadata.Arch)
		obtainedVersionStrings[i] = s
	}
	c.Assert(obtainedVersionStrings, tc.DeepEquals, versionStrings)

	metadata = toolstesting.ParseMetadataFromDir(c, metadataDir, "released", false)
	c.Assert(metadata, tc.HasLen, len(currentVersionStrings))
	obtainedVersionStrings = make([]string, len(currentVersionStrings))
	for i, metadata := range metadata {
		s := fmt.Sprintf("%s-%s-%s", metadata.Version, metadata.Release, metadata.Arch)
		obtainedVersionStrings[i] = s
	}
	c.Assert(obtainedVersionStrings, tc.DeepEquals, currentVersionStrings)

	toolstesting.MakeTools(c, metadataDir, "released", versionStrings)
	metadata = toolstesting.ParseMetadataFromDir(c, metadataDir, "released", false)
	c.Assert(metadata, tc.HasLen, len(versionStrings))
	obtainedVersionStrings = make([]string, len(versionStrings))
	for i, metadata := range metadata {
		s := fmt.Sprintf("%s-%s-%s", metadata.Version, metadata.Release, metadata.Arch)
		obtainedVersionStrings[i] = s
	}
	c.Assert(obtainedVersionStrings, tc.DeepEquals, versionStrings)
}

func (s *GenerateAgentsSuite) TestGenerateDeleteExisting(c *tc.C) {
	metadataDir := c.MkDir()
	toolstesting.MakeTools(c, metadataDir, "proposed", versionStrings)
	toolstesting.MakeTools(c, metadataDir, "released", currentVersionStrings)

	ctx := cmdtesting.Context(c)
	code := cmd.Main(newGenerateAgentsCommandForTests(), ctx, []string{"-d", metadataDir, "--stream", "proposed"})
	c.Assert(code, tc.Equals, 0)
	code = cmd.Main(newGenerateAgentsCommandForTests(), ctx, []string{"-d", metadataDir, "--stream", "released"})
	c.Assert(code, tc.Equals, 0)

	// Remove existing proposed tarballs, and create some different ones.
	err := os.RemoveAll(filepath.Join(metadataDir, "tools", "proposed"))
	c.Assert(err, tc.ErrorIsNil)
	toolstesting.MakeTools(c, metadataDir, "proposed", currentVersionStrings)

	// Generate proposed metadata again, using --clean.
	code = cmd.Main(newGenerateAgentsCommandForTests(), ctx, []string{"-d", metadataDir, "--stream", "proposed", "--clean"})
	c.Assert(code, tc.Equals, 0)

	// Proposed metadata should just list the tarballs that were there, not the merged set.
	metadata := toolstesting.ParseMetadataFromDir(c, metadataDir, "proposed", false)
	c.Assert(metadata, tc.HasLen, len(currentVersionStrings))
	obtainedVersionStrings := make([]string, len(currentVersionStrings))
	for i, metadata := range metadata {
		s := fmt.Sprintf("%s-%s-%s", metadata.Version, metadata.Release, metadata.Arch)
		obtainedVersionStrings[i] = s
	}
	c.Assert(obtainedVersionStrings, tc.DeepEquals, currentVersionStrings)

	// Released metadata should be untouched.
	metadata = toolstesting.ParseMetadataFromDir(c, metadataDir, "released", false)
	c.Assert(metadata, tc.HasLen, len(currentVersionStrings))
	obtainedVersionStrings = make([]string, len(currentVersionStrings))
	for i, metadata := range metadata {
		s := fmt.Sprintf("%s-%s-%s", metadata.Version, metadata.Release, metadata.Arch)
		obtainedVersionStrings[i] = s
	}
	c.Assert(obtainedVersionStrings, tc.DeepEquals, currentVersionStrings)
}

func (s *GenerateAgentsSuite) TestGenerateWithPublicFallback(c *tc.C) {
	// Write tools and metadata to the public tools location.
	toolstesting.MakeToolsWithCheckSum(c, s.publicStorageDir, "released", versionStrings)

	// Run the command with no local metadata.
	ctx := cmdtesting.Context(c)
	metadataDir := c.MkDir()
	code := cmd.Main(newGenerateAgentsCommandForTests(), ctx, []string{"-d", metadataDir, "--stream", "released"})
	c.Assert(code, tc.Equals, 0)
	metadata := toolstesting.ParseMetadataFromDir(c, metadataDir, "released", false)
	c.Assert(metadata, tc.HasLen, len(versionStrings))
	obtainedVersionStrings := make([]string, len(versionStrings))
	for i, metadata := range metadata {
		s := fmt.Sprintf("%s-%s-%s", metadata.Version, metadata.Release, metadata.Arch)
		obtainedVersionStrings[i] = s
	}
	c.Assert(obtainedVersionStrings, tc.DeepEquals, versionStrings)
}

func (s *GenerateAgentsSuite) TestGenerateWithMirrors(c *tc.C) {

	metadataDir := c.MkDir()
	toolstesting.MakeTools(c, metadataDir, "released", versionStrings)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(newGenerateAgentsCommandForTests(), ctx, []string{"--public", "-d", metadataDir, "--stream", "released"})
	c.Assert(code, tc.Equals, 0)
	output := ctx.Stdout.(*bytes.Buffer).String()

	mirrosTmpl := expectedOutputCommon + `
.*Writing tools/streams/v1/index2\.json
.*Writing tools/streams/v1/index\.json
.*Writing tools/streams/v1/com\.ubuntu\.juju-{{.Stream}}-agents\.json
.*Writing tools/streams/v1/mirrors\.json
`
	expectedOutput := makeExpectedOutput(mirrosTmpl, "released", "released")
	c.Check(output, tc.Matches, expectedOutput)
	metadata := toolstesting.ParseMetadataFromDir(c, metadataDir, "released", true)
	c.Check(metadata, tc.HasLen, len(versionStrings))
	obtainedVersionStrings := make([]string, len(versionStrings))
	for i, metadata := range metadata {
		s := fmt.Sprintf("%s-%s-%s", metadata.Version, metadata.Release, metadata.Arch)
		obtainedVersionStrings[i] = s
	}
	c.Assert(obtainedVersionStrings, tc.DeepEquals, versionStrings)
}

func (s *GenerateAgentsSuite) TestNoTools(c *tc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("Skipping on windows, test only set up for Linux tools")
	}
	ctx := cmdtesting.Context(c)
	code := cmd.Main(newGenerateAgentsCommandForTests(), ctx, nil)
	c.Assert(code, tc.Equals, 1)
	stdout := ctx.Stdout.(*bytes.Buffer).String()
	c.Assert(stdout, tc.Matches, ".*Finding agent binaries in .*\n")
	stderr := ctx.Stderr.(*bytes.Buffer).String()
	c.Assert(stderr, tc.Matches, "ERROR no agent binaries available\n")
}

func (s *GenerateAgentsSuite) TestPatchLevels(c *tc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("Skipping on windows, test only set up for Linux tools")
	}
	currentVersion := jujuversion.Current.ToPatch()
	versionStrings := []string{
		currentVersion.String() + "-ubuntu-amd64",
		currentVersion.String() + ".1-ubuntu-amd64",
	}
	metadataDir := osenv.JujuXDGDataHomeDir() // default metadata dir
	toolstesting.MakeTools(c, metadataDir, "released", versionStrings)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(newGenerateAgentsCommandForTests(), ctx, []string{"--stream", "released"})
	c.Assert(code, tc.Equals, 0)
	output := ctx.Stdout.(*bytes.Buffer).String()
	expectedOutput := fmt.Sprintf(`
Finding agent binaries in .*
.*Fetching agent binaries from dir "released" to generate hash: %s
.*Fetching agent binaries from dir "released" to generate hash: %s
.*Writing tools/streams/v1/index2\.json
.*Writing tools/streams/v1/index\.json
.*Writing tools/streams/v1/com\.ubuntu\.juju-released-agents\.json
`[1:], regexp.QuoteMeta(versionStrings[0]), regexp.QuoteMeta(versionStrings[1]))
	c.Assert(output, tc.Matches, expectedOutput)
	metadata := toolstesting.ParseMetadataFromDir(c, metadataDir, "released", false)
	c.Assert(metadata, tc.HasLen, 2)

	filename := fmt.Sprintf("juju-%s-ubuntu-amd64.tgz", currentVersion)
	size, sha256 := toolstesting.SHA256sum(c, filepath.Join(metadataDir, "tools", "released", filename))
	c.Assert(metadata[0], tc.DeepEquals, &tools.ToolsMetadata{
		Release:  "ubuntu",
		Version:  currentVersion.String(),
		Arch:     "amd64",
		Size:     size,
		Path:     "released/" + filename,
		FileType: "tar.gz",
		SHA256:   sha256,
	})

	filename = fmt.Sprintf("juju-%s.1-ubuntu-amd64.tgz", currentVersion)
	size, sha256 = toolstesting.SHA256sum(c, filepath.Join(metadataDir, "tools", "released", filename))
	c.Assert(metadata[1], tc.DeepEquals, &tools.ToolsMetadata{
		Release:  "ubuntu",
		Version:  currentVersion.String() + ".1",
		Arch:     "amd64",
		Size:     size,
		Path:     "released/" + filename,
		FileType: "tar.gz",
		SHA256:   sha256,
	})
}

func (s *GenerateAgentsSuite) TestToolsDataSourceHasKey(c *tc.C) {
	ss := simplestreams.NewSimpleStreams(sstestings.TestDataSourceFactory())
	ds := makeDataSources(ss, "test.me")
	// This data source does not require to contain signed data.
	// However, it may still contain it.
	// Since we will always try to read signed data first,
	// we want to be able to try to read this signed data
	// with public key with Juju-known public key for tools.
	// Bugs #1542127, #1542131
	c.Assert(ds[0].PublicSigningKey(), tc.DeepEquals, keys.JujuPublicKey)
}
