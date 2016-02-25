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

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

type ImageMetadataSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	environ []string
	dir     string
	store   *jujuclienttesting.MemStore
}

var _ = gc.Suite(&ImageMetadataSuite{})

func (s *ImageMetadataSuite) SetUpSuite(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpSuite(c)
	s.environ = os.Environ()
}

func (s *ImageMetadataSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.dir = c.MkDir()
	cacheTestEnvConfig(c)

	err := modelcmd.WriteCurrentController("testing")
	c.Assert(err, jc.ErrorIsNil)
	s.store = jujuclienttesting.NewMemStore()
	s.store.Controllers["testing"] = jujuclient.ControllerDetails{}
	s.store.Accounts["testing"] = &jujuclient.ControllerAccounts{
		CurrentAccount: "admin@local",
	}

	s.PatchEnvironment("AWS_ACCESS_KEY_ID", "access")
	s.PatchEnvironment("AWS_SECRET_ACCESS_KEY", "secret")
}

func runImageMetadata(c *gc.C, store jujuclient.ClientStore, args ...string) (*cmd.Context, error) {
	cmd := &imageMetadataCommand{}
	cmd.SetClientStore(store)
	return testing.RunCommand(c, modelcmd.Wrap(cmd), args...)
}

var seriesVersions map[string]string = map[string]string{
	"precise": "12.04",
	"raring":  "13.04",
	"trusty":  "14.04",
	"xenial":  "16.04",
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
	ctx, err := runImageMetadata(c, s.store,
		"-d", s.dir, "-i", "1234", "-r", "region", "-a", "arch", "-u", "endpoint",
		"-s", "raring", "--virt-type=pv", "--storage=root",
	)
	c.Assert(err, jc.ErrorIsNil)
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
	ctx, err := runImageMetadata(c, s.store,
		"-d", s.dir, "-i", "1234", "-r", "region", "-u", "endpoint", "-s", "raring",
	)
	c.Assert(err, jc.ErrorIsNil)
	out := testing.Stdout(ctx)
	expected := expectedMetadata{
		series: "raring",
		arch:   "amd64",
	}
	s.assertCommandOutput(c, expected, out, defaultIndexFileName, defaultImageFileName)
}

func (s *ImageMetadataSuite) TestImageMetadataFilesLatestLts(c *gc.C) {
	ec2Config, err := config.New(config.UseDefaults, map[string]interface{}{
		"name":   "ec2-latest-lts",
		"type":   "ec2",
		"region": "us-east-1",
	})
	c.Assert(err, jc.ErrorIsNil)

	store, err := configstore.Default()
	c.Assert(err, jc.ErrorIsNil)

	info := store.CreateInfo("ec2-latest-lts")
	c.Assert(err, jc.ErrorIsNil)
	info.SetBootstrapConfig(ec2Config.AllAttrs())
	err = info.Write()
	c.Assert(err, jc.ErrorIsNil)

	ctx, err := runImageMetadata(c, s.store,
		"-m", "ec2-latest-lts",
		"-d", s.dir, "-i", "1234", "-r", "region", "-a", "arch", "-u", "endpoint",
	)
	c.Assert(err, jc.ErrorIsNil)
	out := testing.Stdout(ctx)
	expected := expectedMetadata{
		series: config.LatestLtsSeries(),
		arch:   "arch",
	}
	s.assertCommandOutput(c, expected, out, defaultIndexFileName, defaultImageFileName)
}

func (s *ImageMetadataSuite) TestImageMetadataFilesUsingEnv(c *gc.C) {
	ctx, err := runImageMetadata(c, s.store,
		"-d", s.dir, "-m", "ec2", "-i", "1234", "--virt-type=pv", "--storage=root",
	)
	c.Assert(err, jc.ErrorIsNil)
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
	ctx, err := runImageMetadata(c, s.store,
		"-d", s.dir, "-m", "ec2", "-r", "us-west-1", "-u", "https://ec2.us-west-1.amazonaws.com", "-i", "1234",
	)
	c.Assert(err, jc.ErrorIsNil)
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
	ctx, err := runImageMetadata(c, s.store,
		"-d", s.dir, "-m", "azure", "-r", "region", "-u", "endpoint", "-i", "1234",
	)
	c.Assert(err, jc.ErrorIsNil)
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
		// Missing endpoint/region for model with no HasRegion interface
		args: []string{"-i", "1234", "-m", "azure"},
	},
}

func (s *ImageMetadataSuite) TestImageMetadataBadArgs(c *gc.C) {
	for i, t := range errTests {
		c.Logf("test: %d", i)
		_, err := runImageMetadata(c, s.store, t.args...)
		c.Check(err, gc.NotNil, gc.Commentf("test %d: %s", i, t.args))
		dummy.Reset()
	}
}
