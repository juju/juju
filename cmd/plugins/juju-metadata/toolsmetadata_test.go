// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/localstorage"
	"launchpad.net/juju-core/environs/simplestreams"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/environs/tools"
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
	env, err := environs.NewFromName("erewhemos")
	c.Assert(err, gc.IsNil)
	s.env = env
	envtesting.RemoveAllTools(c, s.env)
}

func (s *ToolsMetadataSuite) TearDownTest(c *gc.C) {
	s.home.Restore()
}

func (s *ToolsMetadataSuite) parseMetadata(c *gc.C) []*tools.ToolsMetadata {
	localStorageDir := config.JujuHomePath(filepath.Join("local", "storage"))
	transport := &http.Transport{}
	transport.RegisterProtocol("file", http.NewFileTransport(http.Dir("/")))
	client := &http.Client{Transport: transport}
	defer func(old *http.Client) {
		simplestreams.SetHttpClient(old)
	}(simplestreams.SetHttpClient(client))

	toolsConstraint := tools.NewToolsConstraint(
		version.CurrentNumber().String(),
		simplestreams.LookupParams{
			Arches: []string{"amd64", "arm", "i386"},
		},
	)
	urls := []string{"file://" + localStorageDir}
	indexPath := "tools/" + simplestreams.DefaultIndexPath
	metadata, err := tools.Fetch(urls, indexPath, toolsConstraint, false)
	c.Assert(err, gc.IsNil)
	return metadata
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
Fetching tools to generate hash: http://.*/tools/juju-1\.12\.0-precise-amd64\.tgz
Fetching tools to generate hash: http://.*/tools/juju-1\.12\.0-precise-i386\.tgz
Fetching tools to generate hash: http://.*/tools/juju-1\.12\.0-raring-amd64\.tgz
Fetching tools to generate hash: http://.*/tools/juju-1\.12\.0-raring-i386\.tgz
Fetching tools to generate hash: http://.*/tools/juju-1\.13\.0-precise-amd64\.tgz
`
	f := "Fetching tools to generate hash: http://.*/tools/juju-%s\\.tgz\n"
	for _, v := range currentVersionStrings {
		expected += fmt.Sprintf(f, regexp.QuoteMeta(v))
	}
	return strings.TrimSpace(expected)
}

var expectedOutputStorage = expectedOutputCommon + `
Writing http://.*/tools/streams/v1/index\.json
Writing http://.*/tools/streams/v1/com\.ubuntu\.juju:released:tools\.json
`

func (s *ToolsMetadataSuite) TestGenerateStorage(c *gc.C) {
	storage := s.env.PublicStorage().(environs.Storage)
	for _, versionString := range versionStrings {
		binary := version.MustParseBinary(versionString)
		envtesting.UploadFakeToolsVersion(c, storage, binary)
	}
	ctx := coretesting.Context(c)
	code := cmd.Main(&ToolsMetadataCommand{}, ctx, nil)
	c.Assert(code, gc.Equals, 0)
	output := ctx.Stdout.(*bytes.Buffer).String()
	c.Assert(output, gc.Matches, expectedOutputStorage)
	metadata := s.parseMetadata(c)
	_ = metadata
	// FIXME(axw) this doesn't work; check with wallyworld how
	// to fetch all metadata. It only returns data for the
	// first matching arch.
	//c.Assert(metadata, gc.HasLen, len(currentVersionStrings))
}

var expectedOutputDirectory = expectedOutputCommon + `
Writing %s/tools/streams/v1/index\.json
Writing %s/tools/streams/v1/com\.ubuntu\.juju:released:tools\.json
`

func (s *ToolsMetadataSuite) TestGenerateDirectory(c *gc.C) {
	storageDir := c.MkDir()
	listener, err := localstorage.Serve("127.0.0.1:0", storageDir)
	c.Assert(err, gc.IsNil)
	defer listener.Close()
	storage := localstorage.Client(listener.Addr().String())
	for _, versionString := range versionStrings {
		binary := version.MustParseBinary(versionString)
		envtesting.UploadFakeToolsVersion(c, storage, binary)
	}
	ctx := coretesting.Context(c)
	code := cmd.Main(&ToolsMetadataCommand{}, ctx, []string{"-d", storageDir})
	c.Assert(code, gc.Equals, 0)
	output := ctx.Stdout.(*bytes.Buffer).String()
	expected := fmt.Sprintf(expectedOutputDirectory, storageDir, storageDir)
	c.Assert(output, gc.Matches, expected)
	metadata := s.parseMetadata(c)
	_ = metadata
	// FIXME(axw) this doesn't work; check with wallyworld how
	// to fetch all metadata. It only returns data for the
	// first matching arch.
	//c.Assert(metadata, gc.HasLen, len(currentVersionStrings))
}

func (s *ToolsMetadataSuite) TestNoTools(c *gc.C) {
	ctx := coretesting.Context(c)
	code := cmd.Main(&ToolsMetadataCommand{}, ctx, nil)
	c.Assert(code, gc.Equals, 1)
	stdout := ctx.Stdout.(*bytes.Buffer).String()
	c.Assert(stdout, gc.Matches, "Finding tools\\.\\.\\.\n")
	stderr := ctx.Stderr.(*bytes.Buffer).String()
	c.Assert(stderr, gc.Matches, "error: no tools available\n")
}

func (s *ToolsMetadataSuite) TestPatchLevels(c *gc.C) {
	currentVersion := version.CurrentNumber()
	currentVersion.Build = 0
	patchVersion := currentVersion
	patchVersion.Build = 1
	versionStrings := [...]string{
		currentVersion.String() + "-precise-amd64",
		currentVersion.String() + ".1-precise-amd64",
	}
	storage := s.env.PublicStorage().(environs.Storage)
	for _, versionString := range versionStrings {
		binary := version.MustParseBinary(versionString)
		envtesting.UploadFakeToolsVersion(c, storage, binary)
	}
	ctx := coretesting.Context(c)
	code := cmd.Main(&ToolsMetadataCommand{}, ctx, nil)
	c.Assert(code, gc.Equals, 0)
	output := ctx.Stdout.(*bytes.Buffer).String()
	expectedOutput := fmt.Sprintf(`
Finding tools\.\.\.
Fetching tools to generate hash: http://.*/tools/juju-%s\.tgz
Fetching tools to generate hash: http://.*/tools/juju-%s\.tgz
Writing http://.*/tools/streams/v1/index\.json
Writing http://.*/tools/streams/v1/com\.ubuntu\.juju:released:tools\.json
`[1:], regexp.QuoteMeta(versionStrings[0]), regexp.QuoteMeta(versionStrings[1]))
	c.Assert(output, gc.Matches, expectedOutput)
	metadata := s.parseMetadata(c)
	_ = metadata
	// FIXME(axw) doesn't work.
	//c.Assert(metadata, gc.HasLen, 2)
}
