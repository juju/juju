// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
)

type ImageMetadataSuite struct {
	testbase.LoggingSuite
	environ []string
	home    *testing.FakeHome
	dir     string
}

var _ = gc.Suite(&ImageMetadataSuite{})

func (s *ImageMetadataSuite) SetUpSuite(c *gc.C) {
	s.LoggingSuite.SetUpSuite(c)
	s.environ = os.Environ()
}

func (s *ImageMetadataSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	os.Clearenv()
	s.dir = c.MkDir()
	// Create a fake certificate so azure test environment can be opened.
	certfile, err := ioutil.TempFile(s.dir, "")
	c.Assert(err, gc.IsNil)
	filename := certfile.Name()
	err = ioutil.WriteFile(filename, []byte("test certificate"), 0644)
	c.Assert(err, gc.IsNil)
	envConfig := strings.Replace(metadataTestEnvConfig, "/home/me/azure.pem", filename, -1)
	s.home = testing.MakeFakeHome(c, envConfig)
	s.PatchEnvironment("AWS_ACCESS_KEY_ID", "access")
	s.PatchEnvironment("AWS_SECRET_ACCESS_KEY", "secret")
}

func (s *ImageMetadataSuite) TearDownTest(c *gc.C) {
	for _, envstring := range s.environ {
		kv := strings.SplitN(envstring, "=", 2)
		os.Setenv(kv[0], kv[1])
	}
	s.home.Restore()
	s.LoggingSuite.TearDownTest(c)
}

var seriesVersions map[string]string = map[string]string{
	"precise": "12.04",
	"raring":  "13.04",
}

type expectedMetadata struct {
	series   string
	arch     string
	region   string
	endpoint string
}

func (s *ImageMetadataSuite) assertCommandOutput(c *gc.C, expected expectedMetadata, errOut, indexFileName, imageFileName string) {
	if expected.region == "" {
		expected.region = "region"
	}
	if expected.endpoint == "" {
		expected.endpoint = "endpoint"
	}
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, gc.Matches, `image metadata files have been written to.*`)
	indexpath := filepath.Join(s.dir, "images", "streams", "v1", indexFileName)
	data, err := ioutil.ReadFile(indexpath)
	c.Assert(err, gc.IsNil)
	content := string(data)
	var indices interface{}
	err = json.Unmarshal(data, &indices)
	c.Assert(err, gc.IsNil)
	c.Assert(indices.(map[string]interface{})["format"], gc.Equals, "index:1.0")
	prodId := fmt.Sprintf("com.ubuntu.cloud:server:%s:%s", seriesVersions[expected.series], expected.arch)
	c.Assert(content, jc.Contains, prodId)
	c.Assert(content, jc.Contains, fmt.Sprintf(`"region": %q`, expected.region))
	c.Assert(content, jc.Contains, fmt.Sprintf(`"endpoint": %q`, expected.endpoint))
	c.Assert(content, jc.Contains, fmt.Sprintf(`"path": "streams/v1/%s"`, imageFileName))

	imagepath := filepath.Join(s.dir, "images", "streams", "v1", imageFileName)
	data, err = ioutil.ReadFile(imagepath)
	c.Assert(err, gc.IsNil)
	content = string(data)
	var images interface{}
	err = json.Unmarshal(data, &images)
	c.Assert(err, gc.IsNil)
	c.Assert(images.(map[string]interface{})["format"], gc.Equals, "products:1.0")
	c.Assert(content, jc.Contains, prodId)
	c.Assert(content, jc.Contains, `"id": "1234"`)
}

const (
	defaultIndexFileName = "index.json"
	defaultImageFileName = "com.ubuntu.cloud:released:imagemetadata.json"
)

func (s *ImageMetadataSuite) TestImageMetadataFilesNoEnv(c *gc.C) {
	ctx := testing.Context(c)
	code := cmd.Main(
		&ImageMetadataCommand{}, ctx, []string{
			"-d", s.dir, "-i", "1234", "-r", "region", "-a", "arch", "-u", "endpoint", "-s", "raring"})
	c.Assert(code, gc.Equals, 0)
	out := testing.Stdout(ctx)
	expected := expectedMetadata{
		series: "raring",
		arch:   "arch",
	}
	s.assertCommandOutput(c, expected, out, defaultIndexFileName, defaultImageFileName)
}

func (s *ImageMetadataSuite) TestImageMetadataFilesDefaultArch(c *gc.C) {
	ctx := testing.Context(c)
	code := cmd.Main(
		&ImageMetadataCommand{}, ctx, []string{
			"-d", s.dir, "-i", "1234", "-r", "region", "-u", "endpoint", "-s", "raring"})
	c.Assert(code, gc.Equals, 0)
	out := testing.Stdout(ctx)
	expected := expectedMetadata{
		series: "raring",
		arch:   "amd64",
	}
	s.assertCommandOutput(c, expected, out, defaultIndexFileName, defaultImageFileName)
}

func (s *ImageMetadataSuite) TestImageMetadataFilesDefaultSeries(c *gc.C) {
	ctx := testing.Context(c)
	code := cmd.Main(
		&ImageMetadataCommand{}, ctx, []string{
			"-d", s.dir, "-i", "1234", "-r", "region", "-a", "arch", "-u", "endpoint"})
	c.Assert(code, gc.Equals, 0)
	out := testing.Stdout(ctx)
	expected := expectedMetadata{
		series: "precise",
		arch:   "arch",
	}
	s.assertCommandOutput(c, expected, out, defaultIndexFileName, defaultImageFileName)
}

func (s *ImageMetadataSuite) TestImageMetadataFilesUsingEnv(c *gc.C) {
	ctx := testing.Context(c)
	code := cmd.Main(
		&ImageMetadataCommand{}, ctx, []string{"-d", s.dir, "-e", "ec2", "-i", "1234"})
	c.Assert(code, gc.Equals, 0)
	out := testing.Stdout(ctx)
	expected := expectedMetadata{
		series:   "precise",
		arch:     "amd64",
		region:   "us-east-1",
		endpoint: "https://ec2.us-east-1.amazonaws.com",
	}
	s.assertCommandOutput(c, expected, out, defaultIndexFileName, defaultImageFileName)
}

func (s *ImageMetadataSuite) TestImageMetadataFilesUsingEnvWithRegionOverride(c *gc.C) {
	ctx := testing.Context(c)
	code := cmd.Main(
		&ImageMetadataCommand{}, ctx, []string{
			"-d", s.dir, "-e", "ec2", "-r", "us-west-1", "-u", "https://ec2.us-west-1.amazonaws.com", "-i", "1234"})
	c.Assert(code, gc.Equals, 0)
	out := testing.Stdout(ctx)
	expected := expectedMetadata{
		series:   "precise",
		arch:     "amd64",
		region:   "us-west-1",
		endpoint: "https://ec2.us-west-1.amazonaws.com",
	}
	s.assertCommandOutput(c, expected, out, defaultIndexFileName, defaultImageFileName)
}

func (s *ImageMetadataSuite) TestImageMetadataFilesUsingEnvWithNoHasRegion(c *gc.C) {
	ctx := testing.Context(c)
	code := cmd.Main(
		&ImageMetadataCommand{}, ctx, []string{
			"-d", s.dir, "-e", "azure", "-r", "region", "-u", "endpoint", "-i", "1234"})
	c.Assert(code, gc.Equals, 0)
	out := testing.Stdout(ctx)
	expected := expectedMetadata{
		series:   "raring",
		arch:     "amd64",
		region:   "region",
		endpoint: "endpoint",
	}
	s.assertCommandOutput(c, expected, out, defaultIndexFileName, defaultImageFileName)
}

type errTestParams struct {
	args []string
}

var errTests = []errTestParams{
	{
		// Missing image id
		args: []string{"-r", "region", "-a", "arch", "-u", "endpoint", "-s", "precise"},
	},
	{
		// Missing region
		args: []string{"-i", "1234", "-a", "arch", "-u", "endpoint", "-s", "precise"},
	},
	{
		// Missing endpoint
		args: []string{"-i", "1234", "-u", "endpoint", "-a", "arch", "-s", "precise"},
	},
	{
		// Missing endpoint/region for environment with no HasRegion interface
		args: []string{"-i", "1234", "-e", "azure"},
	},
}

func (s *ImageMetadataSuite) TestImageMetadataBadArgs(c *gc.C) {
	defer testing.MakeEmptyFakeHome(c).Restore()
	for i, t := range errTests {
		c.Logf("test: %d", i)
		ctx := testing.Context(c)
		code := cmd.Main(&ImageMetadataCommand{}, ctx, t.args)
		c.Check(code, gc.Equals, 2)
	}
}
