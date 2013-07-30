// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	gc "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/testing"
	"os"
	"strings"
)

type ImageMetadataSuite struct {
	environ []string
}

var _ = gc.Suite(&ImageMetadataSuite{})

func (s *ImageMetadataSuite) SetUpSuite(c *gc.C) {
	s.environ = os.Environ()
}

func (s *ImageMetadataSuite) SetUpTest(c *gc.C) {
	os.Clearenv()
}

func (s *ImageMetadataSuite) TearDownTest(c *gc.C) {
	for _, envstring := range s.environ {
		kv := strings.SplitN(envstring, "=", 2)
		os.Setenv(kv[0], kv[1])
	}
}

var seriesVersions map[string]string = map[string]string{
	"precise": "12.04",
	"raring":  "13.04",
}

func (*ImageMetadataSuite) assertCommandOutput(c *gc.C, errOut, series, arch, indexFileName, imageFileName string) {
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, gc.Matches, `Boilerplate image metadata files.*have been written.*Copy the files.*`)
	indexpath := testing.HomePath(".juju", indexFileName)
	data, err := ioutil.ReadFile(indexpath)
	c.Assert(err, gc.IsNil)
	var indices interface{}
	err = json.Unmarshal(data, &indices)
	c.Assert(err, gc.IsNil)
	c.Assert(indices.(map[string]interface{})["format"], gc.Equals, "index:1.0")
	prodId := fmt.Sprintf("com.ubuntu.cloud:server:%s:%s", seriesVersions[series], arch)
	c.Assert(strings.Contains(string(data), prodId), gc.Equals, true)
	c.Assert(strings.Contains(string(data), `"region": "region"`), gc.Equals, true)
	c.Assert(strings.Contains(string(data), `"endpoint": "endpoint"`), gc.Equals, true)
	c.Assert(strings.Contains(string(data), fmt.Sprintf(`"path": "streams/v1/%s"`, imageFileName)), gc.Equals, true)

	imagepath := testing.HomePath(".juju", imageFileName)
	data, err = ioutil.ReadFile(imagepath)
	c.Assert(err, gc.IsNil)
	var images interface{}
	err = json.Unmarshal(data, &images)
	c.Assert(err, gc.IsNil)
	c.Assert(images.(map[string]interface{})["format"], gc.Equals, "products:1.0")
	c.Assert(strings.Contains(string(data), prodId), gc.Equals, true)
	c.Assert(strings.Contains(string(data), `"id": "1234"`), gc.Equals, true)
}

const (
	defaultIndexFileName = "index.json"
	defaultImageFileName = "imagemetadata.json"
)

func (s *ImageMetadataSuite) TestImageMetadataFilesNoEnv(c *gc.C) {
	defer testing.MakeEmptyFakeHome(c).Restore()

	ctx := testing.Context(c)
	code := cmd.Main(
		&ImageMetadataCommand{}, ctx, []string{"-i", "1234", "-r", "region", "-a", "arch", "-e", "endpoint", "-s", "raring"})
	c.Assert(code, gc.Equals, 0)
	errOut := ctx.Stdout.(*bytes.Buffer).String()
	s.assertCommandOutput(c, errOut, "raring", "arch", defaultIndexFileName, defaultImageFileName)
}

func (s *ImageMetadataSuite) TestImageMetadataFilesWithName(c *gc.C) {
	defer testing.MakeEmptyFakeHome(c).Restore()

	ctx := testing.Context(c)
	code := cmd.Main(
		&ImageMetadataCommand{}, ctx, []string{"-n", "foo", "-i", "1234", "-r", "region", "-a", "arch", "-e", "endpoint", "-s", "raring"})
	c.Assert(code, gc.Equals, 0)
	errOut := ctx.Stdout.(*bytes.Buffer).String()
	s.assertCommandOutput(c, errOut, "raring", "arch", "foo-"+defaultIndexFileName, "foo-"+defaultImageFileName)
}

func (s *ImageMetadataSuite) TestImageMetadataFilesDefaultArch(c *gc.C) {
	defer testing.MakeEmptyFakeHome(c).Restore()

	ctx := testing.Context(c)
	code := cmd.Main(
		&ImageMetadataCommand{}, ctx, []string{"-i", "1234", "-r", "region", "-e", "endpoint", "-s", "raring"})
	c.Assert(code, gc.Equals, 0)
	errOut := ctx.Stdout.(*bytes.Buffer).String()
	s.assertCommandOutput(c, errOut, "raring", "amd64", defaultIndexFileName, defaultImageFileName)
}

func (s *ImageMetadataSuite) TestImageMetadataFilesDefaultSeries(c *gc.C) {
	defer testing.MakeEmptyFakeHome(c).Restore()

	ctx := testing.Context(c)
	code := cmd.Main(
		&ImageMetadataCommand{}, ctx, []string{"-i", "1234", "-r", "region", "-a", "arch", "-e", "endpoint"})
	c.Assert(code, gc.Equals, 0)
	errOut := ctx.Stdout.(*bytes.Buffer).String()
	s.assertCommandOutput(c, errOut, "precise", "arch", defaultIndexFileName, defaultImageFileName)
}

func (s *ImageMetadataSuite) TestImageMetadataFilesUsingEnvRegion(c *gc.C) {
	defer testing.MakeEmptyFakeHome(c).Restore()

	os.Setenv("OS_REGION_NAME", "region")
	ctx := testing.Context(c)
	code := cmd.Main(
		&ImageMetadataCommand{}, ctx, []string{"-i", "1234", "-e", "endpoint"})
	c.Assert(code, gc.Equals, 0)
	errOut := ctx.Stdout.(*bytes.Buffer).String()
	s.assertCommandOutput(c, errOut, "precise", "amd64", defaultIndexFileName, defaultImageFileName)
}

func (s *ImageMetadataSuite) TestImageMetadataFilesUsingEnvEndpoint(c *gc.C) {
	defer testing.MakeEmptyFakeHome(c).Restore()

	os.Setenv("OS_AUTH_URL", "endpoint")
	ctx := testing.Context(c)
	code := cmd.Main(
		&ImageMetadataCommand{}, ctx, []string{"-i", "1234", "-r", "region"})
	c.Assert(code, gc.Equals, 0)
	errOut := ctx.Stdout.(*bytes.Buffer).String()
	s.assertCommandOutput(c, errOut, "precise", "amd64", defaultIndexFileName, defaultImageFileName)
}

type errTestParams struct {
	args []string
}

var errTests = []errTestParams{
	{
		// Missing image id
		args: []string{"-r", "region", "-a", "arch", "-e", "endpoint", "-s", "precise"},
	},
	{
		// Missing region
		args: []string{"-i", "1234", "-a", "arch", "-e", "endpoint", "-s", "precise"},
	},
	{
		// Missing endpoint
		args: []string{"-i", "1234", "-e", "endpoint", "-a", "arch", "-s", "precise"},
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
