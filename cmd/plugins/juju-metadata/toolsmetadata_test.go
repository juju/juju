// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/simplestreams"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/environs/tools"
	_ "launchpad.net/juju-core/provider/dummy"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils/set"
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

func (s *ToolsMetadataSuite) parseMetadata(c *gc.C, metadataDir string) []*tools.ToolsMetadata {
	params := simplestreams.ValueParams{
		DataType:      tools.ContentDownload,
		ValueTemplate: tools.ToolsMetadata{},
	}

	baseURL := "file://" + metadataDir + "/tools"
	transport := &http.Transport{}
	transport.RegisterProtocol("file", http.NewFileTransport(http.Dir("/")))
	old := simplestreams.SetHttpClient(&http.Client{Transport: transport})
	defer simplestreams.SetHttpClient(old)

	const requireSigned = false
	indexPath := simplestreams.DefaultIndexPath + simplestreams.UnsignedSuffix
	indexRef, err := simplestreams.GetIndexWithFormat(baseURL, indexPath, "index:1.0", requireSigned, params)
	c.Assert(err, gc.IsNil)
	c.Assert(indexRef.Indexes, gc.HasLen, 1)

	toolsIndexMetadata := indexRef.Indexes["com.ubuntu.juju:released:tools"]
	c.Assert(toolsIndexMetadata, gc.NotNil)

	data, err := ioutil.ReadFile(filepath.Join(metadataDir, "tools", toolsIndexMetadata.ProductsFilePath))
	c.Assert(err, gc.IsNil)

	url := path.Join(baseURL, toolsIndexMetadata.ProductsFilePath)
	c.Assert(err, gc.IsNil)
	cloudMetadata, err := simplestreams.ParseCloudMetadata(data, "products:1.0", url, tools.ToolsMetadata{})
	c.Assert(err, gc.IsNil)

	toolsMetadataMap := make(map[string]*tools.ToolsMetadata)
	var expectedProductIds set.Strings
	var toolsVersions set.Strings
	for _, mc := range cloudMetadata.Products {
		for _, items := range mc.Items {
			for key, item := range items.Items {
				toolsMetadata := item.(*tools.ToolsMetadata)
				toolsMetadataMap[key] = toolsMetadata
				toolsVersions.Add(key)
				productId := fmt.Sprintf("com.ubuntu.juju:%s:%s", toolsMetadata.Version, toolsMetadata.Arch)
				expectedProductIds.Add(productId)
			}
		}
	}

	// Make sure index's product IDs are all represented in the products metadata.
	sort.Strings(toolsIndexMetadata.ProductIds)
	c.Assert(toolsIndexMetadata.ProductIds, gc.DeepEquals, expectedProductIds.SortedValues())

	toolsMetadata := make([]*tools.ToolsMetadata, len(toolsMetadataMap))
	for i, key := range toolsVersions.SortedValues() {
		toolsMetadata[i] = toolsMetadataMap[key]
	}
	return toolsMetadata
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

var expectedOutputDirectory = expectedOutputCommon + `
Writing %s/tools/streams/v1/index\.json
Writing %s/tools/streams/v1/com\.ubuntu\.juju:released:tools\.json
`

func (s *ToolsMetadataSuite) TestGenerateDefaultDirectory(c *gc.C) {
	storage := s.env.PublicStorage().(environs.Storage)
	for _, versionString := range versionStrings {
		binary := version.MustParseBinary(versionString)
		envtesting.UploadFakeToolsVersion(c, storage, binary)
	}
	ctx := coretesting.Context(c)
	code := cmd.Main(&ToolsMetadataCommand{}, ctx, nil)
	c.Assert(code, gc.Equals, 0)
	output := ctx.Stdout.(*bytes.Buffer).String()
	metadataDir := config.JujuHome()
	expected := fmt.Sprintf(expectedOutputDirectory, metadataDir, metadataDir)
	c.Assert(output, gc.Matches, expected)
	metadata := s.parseMetadata(c, metadataDir)
	c.Assert(metadata, gc.HasLen, len(versionStrings))
	obtainedVersionStrings := make([]string, len(versionStrings))
	for i, metadata := range metadata {
		s := fmt.Sprintf("%s-%s-%s", metadata.Version, metadata.Release, metadata.Arch)
		obtainedVersionStrings[i] = s
	}
	c.Assert(obtainedVersionStrings, gc.DeepEquals, versionStrings)
}

func (s *ToolsMetadataSuite) TestGenerateDirectory(c *gc.C) {
	metadataDir := c.MkDir()
	storage := s.env.PublicStorage().(environs.Storage)
	for _, versionString := range versionStrings {
		binary := version.MustParseBinary(versionString)
		envtesting.UploadFakeToolsVersion(c, storage, binary)
	}
	ctx := coretesting.Context(c)
	code := cmd.Main(&ToolsMetadataCommand{}, ctx, []string{"-d", metadataDir})
	c.Assert(code, gc.Equals, 0)
	output := ctx.Stdout.(*bytes.Buffer).String()
	expected := fmt.Sprintf(expectedOutputDirectory, metadataDir, metadataDir)
	c.Assert(output, gc.Matches, expected)
	metadata := s.parseMetadata(c, metadataDir)
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
	metadataDir := config.JujuHome()
	expectedOutput := fmt.Sprintf(`
Finding tools\.\.\.
Fetching tools to generate hash: http://.*/tools/juju-%s\.tgz
Fetching tools to generate hash: http://.*/tools/juju-%s\.tgz
Writing %s/tools/streams/v1/index\.json
Writing %s/tools/streams/v1/com\.ubuntu\.juju:released:tools\.json
`[1:], regexp.QuoteMeta(versionStrings[0]), regexp.QuoteMeta(versionStrings[1]), metadataDir, metadataDir)
	c.Assert(output, gc.Matches, expectedOutput)
	metadata := s.parseMetadata(c, metadataDir)
	c.Assert(metadata, gc.HasLen, 2)
	c.Assert(metadata[0], gc.DeepEquals, &tools.ToolsMetadata{
		Release:  "precise",
		Version:  currentVersion.String(),
		Arch:     "amd64",
		Size:     20,
		Path:     "erewhemos/public/tools/juju-1.13.3-precise-amd64.tgz",
		FileType: "tar.gz",
		SHA256:   "9268ba87201b1514171cc09334db6f680e1e013f5ae584f1b43252c743eea841",
	})
	c.Assert(metadata[1], gc.DeepEquals, &tools.ToolsMetadata{
		Release:  "precise",
		Version:  currentVersion.String() + ".1",
		Arch:     "amd64",
		Size:     22,
		Path:     "erewhemos/public/tools/juju-1.13.3.1-precise-amd64.tgz",
		FileType: "tar.gz",
		SHA256:   "da07c1bb59f18f4426cf6ecadefedee58845f11bab9ed7de8fd68fc90a1bb26f",
	})
}
