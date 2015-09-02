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

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

type ImageMetadataSuite struct {
	testing.FakeJujuHomeSuite
	environ []string
	dir     string
}

var _ = gc.Suite(&ImageMetadataSuite{})

func (s *ImageMetadataSuite) SetUpSuite(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpSuite(c)
	s.environ = os.Environ()
}

var testCert = `
-----BEGIN PRIVATE KEY-----
MIIBCgIBADANBgkqhkiG9w0BAQEFAASB9TCB8gIBAAIxAKQGQxP1i0VfCWn4KmMP
taUFn8sMBKjP/9vHnUYdZRvvmoJCA1C6arBUDp8s2DNX+QIDAQABAjBLRqhwN4dU
LfqHDKJ/Vg1aD8u3Buv4gYRBxdFR5PveyqHSt5eJ4g/x/4ndsvr2OqUCGQDNfNlD
zxHCiEAwZZAPaAkn8jDkFupTljcCGQDMWCujiVZ1NNuBD/N32Yt8P9JDiNzZa08C
GBW7VXLxbExpgnhb1V97vjQmTfthXQjYAwIYSTEjoFXm4+Bk5xuBh2IidgSeGZaC
FFY9AhkAsteo31cyQw2xJ80SWrmsIw+ps7Cvt5W9
-----END PRIVATE KEY-----
-----BEGIN CERTIFICATE-----
MIIBDzCByqADAgECAgkAgIBb3+lSwzEwDQYJKoZIhvcNAQEFBQAwFTETMBEGA1UE
AxQKQEhvc3ROYW1lQDAeFw0xMzA3MTkxNjA1NTRaFw0yMzA3MTcxNjA1NTRaMBUx
EzARBgNVBAMUCkBIb3N0TmFtZUAwTDANBgkqhkiG9w0BAQEFAAM7ADA4AjEApAZD
E/WLRV8JafgqYw+1pQWfywwEqM//28edRh1lG++agkIDULpqsFQOnyzYM1f5AgMB
AAGjDTALMAkGA1UdEwQCMAAwDQYJKoZIhvcNAQEFBQADMQABKfn08tKfzzqMMD2w
PI2fs3bw5bRH8tmGjrsJeEdp9crCBS8I3hKcxCkTTRTowdY=
-----END CERTIFICATE-----
`

func (s *ImageMetadataSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.dir = c.MkDir()
	// Create a fake certificate so azure test environment can be opened.
	certfile, err := ioutil.TempFile(s.dir, "")
	c.Assert(err, jc.ErrorIsNil)
	filename := certfile.Name()
	err = ioutil.WriteFile(filename, []byte(testCert), 0644)
	c.Assert(err, jc.ErrorIsNil)
	envConfig := strings.Replace(metadataTestEnvConfig, "/home/me/azure.pem", filename, -1)
	testing.WriteEnvironments(c, envConfig)
	s.PatchEnvironment("AWS_ACCESS_KEY_ID", "access")
	s.PatchEnvironment("AWS_SECRET_ACCESS_KEY", "secret")
}

var seriesVersions map[string]string = map[string]string{
	"precise": "12.04",
	"raring":  "13.04",
	"trusty":  "14.04",
}

type expectedMetadata struct {
	series   string
	arch     string
	region   string
	endpoint string
	virtType string
	storage  string
}

func (s *ImageMetadataSuite) assertCommandOutput(c *gc.C, expected expectedMetadata, errOut, indexFileName, imageFileName string) {
	if expected.region == "" {
		expected.region = "region"
	}
	if expected.endpoint == "" {
		expected.endpoint = "endpoint"
	}
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, gc.Matches, `Image metadata files have been written to.*`)
	indexpath := filepath.Join(s.dir, "images", "streams", "v1", indexFileName)
	data, err := ioutil.ReadFile(indexpath)
	c.Assert(err, jc.ErrorIsNil)
	content := string(data)
	var indices interface{}
	err = json.Unmarshal(data, &indices)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(indices.(map[string]interface{})["format"], gc.Equals, "index:1.0")
	prodId := fmt.Sprintf("com.ubuntu.cloud:server:%s:%s", seriesVersions[expected.series], expected.arch)
	c.Assert(content, jc.Contains, prodId)
	c.Assert(content, jc.Contains, fmt.Sprintf(`"region": %q`, expected.region))
	c.Assert(content, jc.Contains, fmt.Sprintf(`"endpoint": %q`, expected.endpoint))
	c.Assert(content, jc.Contains, fmt.Sprintf(`"path": "streams/v1/%s"`, imageFileName))

	imagepath := filepath.Join(s.dir, "images", "streams", "v1", imageFileName)
	data, err = ioutil.ReadFile(imagepath)
	c.Assert(err, jc.ErrorIsNil)
	content = string(data)
	var images interface{}
	err = json.Unmarshal(data, &images)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(images.(map[string]interface{})["format"], gc.Equals, "products:1.0")
	c.Assert(content, jc.Contains, prodId)
	c.Assert(content, jc.Contains, `"id": "1234"`)
	if expected.virtType != "" {
		c.Assert(content, jc.Contains, fmt.Sprintf(`"virt": %q`, expected.virtType))
	}
	if expected.storage != "" {
		c.Assert(content, jc.Contains, fmt.Sprintf(`"root_store": %q`, expected.storage))
	}
}

const (
	defaultIndexFileName = "index.json"
	defaultImageFileName = "com.ubuntu.cloud-released-imagemetadata.json"
)

func (s *ImageMetadataSuite) TestImageMetadataFilesNoEnv(c *gc.C) {
	ctx := testing.Context(c)
	code := cmd.Main(
		envcmd.Wrap(&ImageMetadataCommand{}), ctx, []string{
			"-d", s.dir, "-i", "1234", "-r", "region", "-a", "arch", "-u", "endpoint", "-s", "raring", "--virt-type=pv", "--storage=root"})
	c.Assert(code, gc.Equals, 0)
	out := testing.Stdout(ctx)
	expected := expectedMetadata{
		series:   "raring",
		arch:     "arch",
		virtType: "pv",
		storage:  "root",
	}
	s.assertCommandOutput(c, expected, out, defaultIndexFileName, defaultImageFileName)
}

func (s *ImageMetadataSuite) TestImageMetadataFilesDefaultArch(c *gc.C) {
	ctx := testing.Context(c)
	code := cmd.Main(
		envcmd.Wrap(&ImageMetadataCommand{}), ctx, []string{
			"-d", s.dir, "-i", "1234", "-r", "region", "-u", "endpoint", "-s", "raring"})
	c.Assert(code, gc.Equals, 0)
	out := testing.Stdout(ctx)
	expected := expectedMetadata{
		series: "raring",
		arch:   "amd64",
	}
	s.assertCommandOutput(c, expected, out, defaultIndexFileName, defaultImageFileName)
}

func (s *ImageMetadataSuite) TestImageMetadataFilesLatestLts(c *gc.C) {
	envConfig := strings.Replace(metadataTestEnvConfig, "default-series: precise", "", -1)
	testing.WriteEnvironments(c, envConfig)
	ctx := testing.Context(c)
	code := cmd.Main(
		envcmd.Wrap(&ImageMetadataCommand{}), ctx, []string{
			"-d", s.dir, "-i", "1234", "-r", "region", "-a", "arch", "-u", "endpoint"})
	c.Assert(code, gc.Equals, 0)
	out := testing.Stdout(ctx)
	expected := expectedMetadata{
		series: config.LatestLtsSeries(),
		arch:   "arch",
	}
	s.assertCommandOutput(c, expected, out, defaultIndexFileName, defaultImageFileName)
}

func (s *ImageMetadataSuite) TestImageMetadataFilesUsingEnv(c *gc.C) {
	ctx := testing.Context(c)
	code := cmd.Main(
		envcmd.Wrap(&ImageMetadataCommand{}), ctx, []string{"-d", s.dir, "-e", "ec2", "-i", "1234", "--virt-type=pv", "--storage=root"})
	c.Assert(code, gc.Equals, 0)
	out := testing.Stdout(ctx)
	expected := expectedMetadata{
		series:   "precise",
		arch:     "amd64",
		region:   "us-east-1",
		endpoint: "https://ec2.us-east-1.amazonaws.com",
		virtType: "pv",
		storage:  "root",
	}
	s.assertCommandOutput(c, expected, out, defaultIndexFileName, defaultImageFileName)
}

func (s *ImageMetadataSuite) TestImageMetadataFilesUsingEnvWithRegionOverride(c *gc.C) {
	ctx := testing.Context(c)
	code := cmd.Main(
		envcmd.Wrap(&ImageMetadataCommand{}), ctx, []string{
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
		envcmd.Wrap(&ImageMetadataCommand{}), ctx, []string{
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
	testing.MakeSampleJujuHome(c)
	s.AddCleanup(func(*gc.C) {
		dummy.Reset()
	})
	for i, t := range errTests {
		c.Logf("test: %d", i)
		ctx := testing.Context(c)
		code := cmd.Main(envcmd.Wrap(&ImageMetadataCommand{}), ctx, t.args)
		c.Check(code, gc.Equals, 1)
	}
}
