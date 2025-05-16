// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/version"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
)

type ImageMetadataSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	environ []string
	dir     string
	store   *jujuclient.MemStore
}

func TestImageMetadataSuite(t *stdtesting.T) { tc.Run(t, &ImageMetadataSuite{}) }
func (s *ImageMetadataSuite) SetUpSuite(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpSuite(c)
	s.environ = os.Environ()
}

func (s *ImageMetadataSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.dir = c.MkDir()

	s.store = jujuclienttesting.MinimalStore()
	cacheTestEnvConfig(c, s.store)

	s.PatchEnvironment("AWS_ACCESS_KEY_ID", "access")
	s.PatchEnvironment("AWS_SECRET_ACCESS_KEY", "secret")
}

func runImageMetadata(c *tc.C, store jujuclient.ClientStore, args ...string) (*cmd.Context, error) {
	cmd := &imageMetadataCommand{}
	cmd.SetClientStore(store)
	return cmdtesting.RunCommand(c, modelcmd.WrapController(cmd), args...)
}

type expectedMetadata struct {
	version  string
	arch     string
	region   string
	endpoint string
	virtType string
	storage  string
}

func (s *ImageMetadataSuite) assertCommandOutput(c *tc.C, expected expectedMetadata, errOut, indexFileName, imageFileName string) {
	if expected.region == "" {
		expected.region = "region"
	}
	if expected.endpoint == "" {
		expected.endpoint = "endpoint"
	}
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, tc.Matches, `Image metadata files have been written to.*`)
	indexpath := filepath.Join(s.dir, "images", "streams", "v1", indexFileName)
	data, err := os.ReadFile(indexpath)
	c.Assert(err, tc.ErrorIsNil)
	content := string(data)
	var indices interface{}
	err = json.Unmarshal(data, &indices)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(indices.(map[string]interface{})["format"], tc.Equals, "index:1.0")
	prodId := fmt.Sprintf("com.ubuntu.cloud:server:%s:%s", expected.version, expected.arch)
	c.Assert(content, tc.Contains, prodId)
	c.Assert(content, tc.Contains, fmt.Sprintf(`"region": %q`, expected.region))
	c.Assert(content, tc.Contains, fmt.Sprintf(`"endpoint": %q`, expected.endpoint))
	c.Assert(content, tc.Contains, fmt.Sprintf(`"path": "streams/v1/%s"`, imageFileName))

	imagepath := filepath.Join(s.dir, "images", "streams", "v1", imageFileName)
	data, err = os.ReadFile(imagepath)
	c.Assert(err, tc.ErrorIsNil)
	content = string(data)
	var images interface{}
	err = json.Unmarshal(data, &images)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(images.(map[string]interface{})["format"], tc.Equals, "products:1.0")
	c.Assert(content, tc.Contains, prodId)
	c.Assert(content, tc.Contains, `"id": "1234"`)
	if expected.virtType != "" {
		c.Assert(content, tc.Contains, fmt.Sprintf(`"virt": %q`, expected.virtType))
	}
	if expected.storage != "" {
		c.Assert(content, tc.Contains, fmt.Sprintf(`"root_store": %q`, expected.storage))
	}
}

const (
	defaultIndexFileName = "index.json"
	defaultImageFileName = "com.ubuntu.cloud-released-imagemetadata.json"
)

func (s *ImageMetadataSuite) TestImageMetadataFilesNoEnv(c *tc.C) {
	ctx, err := runImageMetadata(c, s.store,
		"-d", s.dir, "-i", "1234", "-r", "region", "-a", "arch", "-u", "endpoint",
		"--base", "ubuntu@13.04", "--virt-type=pv", "--storage=root",
	)
	c.Assert(err, tc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	expected := expectedMetadata{
		version:  "13.04",
		arch:     "arch",
		virtType: "pv",
		storage:  "root",
	}
	s.assertCommandOutput(c, expected, out, defaultIndexFileName, defaultImageFileName)
}

func (s *ImageMetadataSuite) TestImageMetadataFilesDefaultArch(c *tc.C) {
	ctx, err := runImageMetadata(c, s.store,
		"-d", s.dir, "-i", "1234", "-r", "region", "-u", "endpoint", "--base", "ubuntu@13.04",
	)
	c.Assert(err, tc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	expected := expectedMetadata{
		version: "13.04",
		arch:    "amd64",
	}
	s.assertCommandOutput(c, expected, out, defaultIndexFileName, defaultImageFileName)
}

func (s *ImageMetadataSuite) TestImageMetadataFilesLatestLTS(c *tc.C) {
	ec2Config, err := config.New(config.UseDefaults, map[string]interface{}{
		"name":            "ec2-latest-lts",
		"type":            "ec2",
		"uuid":            testing.ModelTag.Id(),
		"controller-uuid": testing.ControllerTag.Id(),
		"region":          "us-east-1",
	})
	c.Assert(err, tc.ErrorIsNil)
	s.store.BootstrapConfig["ec2-controller"] = jujuclient.BootstrapConfig{
		ControllerConfig: testing.FakeControllerConfig(),
		Cloud:            "ec2",
		CloudRegion:      "us-east-1",
		Config:           ec2Config.AllAttrs(),
	}

	ctx, err := runImageMetadata(c, s.store,
		"-c", "ec2-controller",
		"-d", s.dir, "-i", "1234", "-r", "region", "-a", "arch", "-u", "endpoint",
	)
	c.Assert(err, tc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	expected := expectedMetadata{
		version: version.DefaultSupportedLTSBase().Channel.Track,
		arch:    "arch",
	}
	s.assertCommandOutput(c, expected, out, defaultIndexFileName, defaultImageFileName)
}

func (s *ImageMetadataSuite) TestImageMetadataFilesUsingEnv(c *tc.C) {
	ctx, err := runImageMetadata(c, s.store,
		"-d", s.dir, "-c", "ec2-controller", "-i", "1234", "--virt-type=pv", "--storage=root",
	)
	c.Assert(err, tc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	expected := expectedMetadata{
		version:  "22.04",
		arch:     "amd64",
		region:   "us-east-1",
		endpoint: "https://ec2.us-east-1.amazonaws.com",
		virtType: "pv",
		storage:  "root",
	}
	s.assertCommandOutput(c, expected, out, defaultIndexFileName, defaultImageFileName)
}

func (s *ImageMetadataSuite) TestImageMetadataFilesUsingEnvWithoutUsingBase(c *tc.C) {
	ctx, err := runImageMetadata(c, s.store,
		"-d", s.dir, "-c", "ec2-controller", "-i", "1234", "--virt-type=pv", "--storage=root", "--base=ubuntu@20.04",
	)
	c.Assert(err, tc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	expected := expectedMetadata{
		version:  "20.04",
		arch:     "amd64",
		region:   "us-east-1",
		endpoint: "https://ec2.us-east-1.amazonaws.com",
		virtType: "pv",
		storage:  "root",
	}
	s.assertCommandOutput(c, expected, out, defaultIndexFileName, defaultImageFileName)
}

func (s *ImageMetadataSuite) TestImageMetadataFilesUsingEnvWithRegionOverride(c *tc.C) {
	ctx, err := runImageMetadata(c, s.store,
		"-d", s.dir, "-c", "ec2-controller", "-r", "us-west-1", "-u", "https://ec2.us-west-1.amazonaws.com", "-i", "1234",
	)
	c.Assert(err, tc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	expected := expectedMetadata{
		version:  "22.04",
		arch:     "amd64",
		region:   "us-west-1",
		endpoint: "https://ec2.us-west-1.amazonaws.com",
	}
	s.assertCommandOutput(c, expected, out, defaultIndexFileName, defaultImageFileName)
}

type errTestParams struct {
	args []string
}

var errTests = []errTestParams{
	{
		// Missing image id
		args: []string{"-r", "region", "-a", "arch", "-u", "endpoint", "-base", "ubuntu@12.04"},
	},
	{
		// Missing region
		args: []string{"-i", "1234", "-a", "arch", "-u", "endpoint", "-base", "ubuntu@12.04"},
	},
	{
		// Missing endpoint
		args: []string{"-i", "1234", "-u", "endpoint", "-a", "arch", "-base", "ubuntu@12.04"},
	},
}

func (s *ImageMetadataSuite) TestImageMetadataBadArgs(c *tc.C) {
	for i, t := range errTests {
		c.Logf("test: %d", i)
		_, err := runImageMetadata(c, s.store, t.args...)
		c.Check(err, tc.NotNil, tc.Commentf("test %d: %s", i, t.args))
	}
}
