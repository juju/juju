// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"strings"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	"gopkg.in/amz.v3/aws"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	coretesting "github.com/juju/juju/testing"
)

type ValidateImageMetadataSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	metadataDir string
	store       *jujuclienttesting.MemStore
}

var _ = gc.Suite(&ValidateImageMetadataSuite{})

func runValidateImageMetadata(c *gc.C, store jujuclient.ClientStore, args ...string) (*cmd.Context, error) {
	cmd := &validateImageMetadataCommand{}
	cmd.SetClientStore(store)
	return coretesting.RunCommand(c, modelcmd.Wrap(cmd), args...)
}

var validateInitImageErrorTests = []struct {
	args []string
	err  string
}{
	{
		args: []string{"-p", "ec2", "-r", "region", "-d", "dir"},
		err:  `series required if provider type is specified`,
	}, {
		args: []string{"-p", "ec2", "-s", "series", "-d", "dir"},
		err:  `region required if provider type is specified`,
	}, {
		args: []string{"-p", "ec2", "-s", "series", "-r", "region"},
		err:  `metadata directory required if provider type is specified`,
	},
}

func (s *ValidateImageMetadataSuite) TestInitErrors(c *gc.C) {
	for i, t := range validateInitImageErrorTests {
		c.Logf("test %d", i)
		err := coretesting.InitCommand(newValidateImageMetadataCommand(), t.args)
		c.Check(err, gc.ErrorMatches, t.err)
	}
}

func (s *ValidateImageMetadataSuite) TestInvalidProviderError(c *gc.C) {
	_, err := runValidateImageMetadata(c, s.store, "-p", "foo", "-s", "series", "-r", "region", "-d", "dir")
	c.Check(err, gc.ErrorMatches, `no registered provider for "foo"`)
}

func (s *ValidateImageMetadataSuite) TestUnsupportedProviderError(c *gc.C) {
	_, err := runValidateImageMetadata(c, s.store, "-p", "maas", "-s", "series", "-r", "region", "-d", "dir")
	c.Check(err, gc.ErrorMatches, `maas provider does not support image metadata validation`)
}

func (s *ValidateImageMetadataSuite) makeLocalMetadata(c *gc.C, id, region, series, endpoint, stream string) error {
	im := &imagemetadata.ImageMetadata{
		Id:     id,
		Arch:   "amd64",
		Stream: stream,
	}
	cloudSpec := simplestreams.CloudSpec{
		Region:   region,
		Endpoint: endpoint,
	}
	targetStorage, err := filestorage.NewFileStorageWriter(s.metadataDir)
	if err != nil {
		return err
	}
	err = imagemetadata.MergeAndWriteMetadata(series, []*imagemetadata.ImageMetadata{im}, &cloudSpec, targetStorage)
	if err != nil {
		return err
	}
	return nil
}

func cacheTestEnvConfig(c *gc.C) {
	ec2Config, err := config.New(config.UseDefaults, map[string]interface{}{
		"name":           "ec2",
		"type":           "ec2",
		"default-series": "precise",
		"region":         "us-east-1",
	})
	c.Assert(err, jc.ErrorIsNil)

	azureConfig, err := config.New(config.UseDefaults, map[string]interface{}{
		"name":                      "azure",
		"type":                      "azure",
		"default-series":            "raring",
		"location":                  "West US",
		"endpoint":                  "https://management.azure.com",
		"storage-endpoint":          "https://core.windows.net",
		"subscription-id":           "foo",
		"application-id":            "bar",
		"application-password":      "baz",
		"tenant-id":                 "qux",
		"controller-resource-group": "fnord",
	})
	c.Assert(err, jc.ErrorIsNil)

	store, err := configstore.Default()
	c.Assert(err, jc.ErrorIsNil)

	ec2 := store.CreateInfo("ec2")
	c.Assert(err, jc.ErrorIsNil)
	ec2.SetBootstrapConfig(ec2Config.AllAttrs())
	err = ec2.Write()
	c.Assert(err, jc.ErrorIsNil)

	azure := store.CreateInfo("azure")
	c.Assert(err, jc.ErrorIsNil)
	azure.SetBootstrapConfig(azureConfig.AllAttrs())
	err = azure.Write()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ValidateImageMetadataSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.metadataDir = c.MkDir()
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
	// All of the following are recognized as fallbacks by goamz.
	s.PatchEnvironment("AWS_ACCESS_KEY", "")
	s.PatchEnvironment("AWS_SECRET_KEY", "")
	s.PatchEnvironment("EC2_ACCESS_KEY", "")
	s.PatchEnvironment("EC2_SECRET_KEY", "")
}

func (s *ValidateImageMetadataSuite) setupEc2LocalMetadata(c *gc.C, region, stream string) {
	ec2Region, ok := aws.Regions[region]
	if !ok {
		c.Fatalf("unknown ec2 region %q", region)
	}
	endpoint := ec2Region.EC2Endpoint
	s.makeLocalMetadata(c, "1234", region, "precise", endpoint, stream)
}

func (s *ValidateImageMetadataSuite) assertEc2LocalMetadataUsingEnvironment(c *gc.C, stream string) {
	s.setupEc2LocalMetadata(c, "us-east-1", stream)
	ctx, err := runValidateImageMetadata(c, s.store, "-m", "ec2", "-d", s.metadataDir, "--stream", stream)
	c.Assert(err, jc.ErrorIsNil)
	stdout := coretesting.Stdout(ctx)
	stderr := coretesting.Stderr(ctx)
	strippedOut := strings.Replace(stdout, "\n", "", -1)
	c.Check(strippedOut, gc.Matches,
		`ImageIds:.*"1234".*Region:.*us-east-1.*Resolve Metadata:.*source: local metadata directory.*`,
	)
	c.Check(stderr, gc.Matches, "")
}

func (s *ValidateImageMetadataSuite) TestEc2LocalMetadataUsingEnvironment(c *gc.C) {
	s.assertEc2LocalMetadataUsingEnvironment(c, "")
	s.assertEc2LocalMetadataUsingEnvironment(c, imagemetadata.ReleasedStream)
	s.assertEc2LocalMetadataUsingEnvironment(c, "daily")
}

func (s *ValidateImageMetadataSuite) TestEc2LocalMetadataUsingIncompleteEnvironment(c *gc.C) {
	s.PatchEnvironment("AWS_ACCESS_KEY_ID", "")
	s.PatchEnvironment("AWS_SECRET_ACCESS_KEY", "")
	s.PatchEnvironment("EC2_ACCESS_KEY", "")
	s.PatchEnvironment("EC2_SECRET_KEY", "")
	s.setupEc2LocalMetadata(c, "us-east-1", "")
	_, err := runValidateImageMetadata(c, s.store, "-m", "ec2", "-d", s.metadataDir)
	c.Assert(err, gc.ErrorMatches, `.*model has no access-key or secret-key`)
}

func (s *ValidateImageMetadataSuite) TestEc2LocalMetadataWithManualParams(c *gc.C) {
	s.setupEc2LocalMetadata(c, "us-west-1", "")
	ctx, err := runValidateImageMetadata(c, s.store,
		"-p", "ec2", "-s", "precise", "-r", "us-west-1",
		"-u", "https://ec2.us-west-1.amazonaws.com", "-d", s.metadataDir,
	)
	c.Assert(err, jc.ErrorIsNil)
	errOut := coretesting.Stdout(ctx)
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(
		strippedOut, gc.Matches,
		`ImageIds:.*"1234".*Region:.*us-west-1.*Resolve Metadata:.*source: local metadata directory.*`)
}

func (s *ValidateImageMetadataSuite) TestEc2LocalMetadataNoMatch(c *gc.C) {
	s.setupEc2LocalMetadata(c, "us-east-1", "")
	_, err := runValidateImageMetadata(c, s.store,
		"-p", "ec2", "-s", "raring", "-r", "us-west-1",
		"-u", "https://ec2.us-west-1.amazonaws.com", "-d", s.metadataDir,
	)
	c.Check(err, gc.ErrorMatches, "(.|\n)*Resolve Metadata:(.|\n)*")
	_, err = runValidateImageMetadata(c, s.store,
		"-p", "ec2", "-s", "precise", "-r", "region",
		"-u", "https://ec2.region.amazonaws.com", "-d", s.metadataDir,
	)
	c.Check(err, gc.ErrorMatches, `unknown region "region"`)
}

func (s *ValidateImageMetadataSuite) TestOpenstackLocalMetadataWithManualParams(c *gc.C) {
	s.makeLocalMetadata(c, "1234", "region-2", "raring", "some-auth-url", "")
	ctx, err := runValidateImageMetadata(c, s.store,
		"-p", "openstack", "-s", "raring", "-r", "region-2",
		"-u", "some-auth-url", "-d", s.metadataDir,
	)
	c.Assert(err, jc.ErrorIsNil)
	errOut := coretesting.Stdout(ctx)
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(
		strippedOut, gc.Matches,
		`ImageIds:.*"1234".*Region:.*region-2.*Resolve Metadata:.*source: local metadata directory.*`)
}

func (s *ValidateImageMetadataSuite) TestOpenstackLocalMetadataNoMatch(c *gc.C) {
	s.makeLocalMetadata(c, "1234", "region-2", "raring", "some-auth-url", "")
	_, err := runValidateImageMetadata(c, s.store,
		"-p", "openstack", "-s", "precise", "-r", "region-2",
		"-u", "some-auth-url", "-d", s.metadataDir,
	)
	c.Check(err, gc.ErrorMatches, "(.|\n)*Resolve Metadata:(.|\n)*")
	_, err = runValidateImageMetadata(c, s.store,
		"-p", "openstack", "-s", "raring", "-r", "region-3",
		"-u", "some-auth-url", "-d", s.metadataDir,
	)
	c.Check(err, gc.ErrorMatches, "(.|\n)*Resolve Metadata:(.|\n)*")
}

func (s *ValidateImageMetadataSuite) TestImagesDataSourceHasKey(c *gc.C) {
	ds := imagesDataSources("test.me")
	// This data source does not require to contain signed data.
	// However, it may still contain it.
	// Since we will always try to read signed data first,
	// we want to be able to try to read this signed data
	// with a user provided public key. For this test, none is provided.
	// Bugs #1542127, #1542131
	c.Assert(ds[0].PublicSigningKey(), gc.Equals, "")
}
