// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/modelcmd"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	sstestings "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	coretesting "github.com/juju/juju/testing"
)

type ValidateImageMetadataSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	metadataDir string
	store       *jujuclient.MemStore
}

var _ = gc.Suite(&ValidateImageMetadataSuite{})

func runValidateImageMetadata(c *gc.C, store jujuclient.ClientStore, args ...string) (*cmd.Context, error) {
	cmd := &validateImageMetadataCommand{}
	cmd.SetClientStore(store)
	return cmdtesting.RunCommand(c, modelcmd.WrapController(cmd), args...)
}

var validateInitImageErrorTests = []struct {
	args []string
	err  string
}{
	{
		args: []string{"-p", "ec2", "-r", "region", "-d", "dir"},
		err:  `base required if provider type is specified`,
	}, {
		args: []string{"-p", "ec2", "--base", "base", "-d", "dir"},
		err:  `region required if provider type is specified`,
	}, {
		args: []string{"-p", "ec2", "--base", "base", "-r", "region"},
		err:  `metadata directory required if provider type is specified`,
	},
}

func (s *ValidateImageMetadataSuite) TestInitErrors(c *gc.C) {
	for i, t := range validateInitImageErrorTests {
		c.Logf("test %d", i)
		cmd := &validateImageMetadataCommand{}
		cmd.SetClientStore(jujuclienttesting.MinimalStore())
		err := cmdtesting.InitCommand(modelcmd.WrapController(cmd), t.args)
		c.Check(err, gc.ErrorMatches, t.err)
	}
}

func (s *ValidateImageMetadataSuite) TestInvalidProviderError(c *gc.C) {
	_, err := runValidateImageMetadata(c, s.store, "-p", "foo", "--base", "ubuntu@22.04", "-r", "region", "-d", "dir")
	c.Check(err, gc.ErrorMatches, `no registered provider for "foo"`)
}

func (s *ValidateImageMetadataSuite) TestUnsupportedProviderError(c *gc.C) {
	_, err := runValidateImageMetadata(c, s.store, "-p", "maas", "--base", "ubuntu@22.04", "-r", "region", "-d", "dir")
	c.Check(err, gc.ErrorMatches, `maas provider does not support image metadata validation`)
}

func (s *ValidateImageMetadataSuite) makeLocalMetadata(id, region string, base corebase.Base, endpoint, stream string) error {
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
	ss := simplestreams.NewSimpleStreams(sstestings.TestDataSourceFactory())
	err = imagemetadata.MergeAndWriteMetadata(context.Background(), ss, base, []*imagemetadata.ImageMetadata{im}, &cloudSpec, targetStorage)
	if err != nil {
		return err
	}
	return nil
}

func cacheTestEnvConfig(c *gc.C, store *jujuclient.MemStore) {
	ec2UUID := utils.MustNewUUID().String()
	ec2Config, err := config.New(config.UseDefaults, map[string]interface{}{
		"name":            "ec2",
		"type":            "ec2",
		"default-base":    "ubuntu@22.04",
		"controller-uuid": coretesting.ControllerTag.Id(),
		"uuid":            ec2UUID,
	})
	c.Assert(err, jc.ErrorIsNil)
	store.Controllers["ec2-controller"] = jujuclient.ControllerDetails{
		ControllerUUID: coretesting.ControllerTag.Id(),
		CACert:         coretesting.CACert,
	}
	store.Controllers["ec2-controller-lts"] = jujuclient.ControllerDetails{
		ControllerUUID: coretesting.ControllerTag.Id(),
		CACert:         coretesting.CACert,
	}
	store.Models["ec2-controller"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{
			"admin/ec2": {
				ModelType: model.IAAS,
			},
			"admin/ec2-latest-lts": {
				ModelType: model.IAAS,
			},
		},
		CurrentModel: "admin/controller",
	}
	store.Accounts["ec2-controller"] = jujuclient.AccountDetails{
		User: "admin",
	}
	store.Accounts["ec2-controller-lts"] = jujuclient.AccountDetails{
		User: "admin",
	}

	store.BootstrapConfig["ec2-controller"] = jujuclient.BootstrapConfig{
		ControllerConfig:    coretesting.FakeControllerConfig(),
		ControllerModelUUID: ec2UUID,
		Config:              ec2Config.AllAttrs(),
		Cloud:               "ec2",
		CloudType:           "ec2",
		CloudRegion:         "us-east-1",
		CloudEndpoint:       "https://ec2.us-east-1.amazonaws.com",
	}
}

func (s *ValidateImageMetadataSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.metadataDir = c.MkDir()

	s.store = jujuclienttesting.MinimalStore()
	cacheTestEnvConfig(c, s.store)

	s.PatchEnvironment("AWS_ACCESS_KEY_ID", "access")
	s.PatchEnvironment("AWS_SECRET_ACCESS_KEY", "secret")
	// All of the following are recognized as fallbacks by goamz.
	s.PatchEnvironment("AWS_ACCESS_KEY", "")
	s.PatchEnvironment("AWS_SECRET_KEY", "")
	s.PatchEnvironment("EC2_ACCESS_KEY", "")
	s.PatchEnvironment("EC2_SECRET_KEY", "")
}

func (s *ValidateImageMetadataSuite) setupEc2LocalMetadata(c *gc.C, region, stream string) {
	resolver := ec2.NewDefaultEndpointResolver()
	ep, err := resolver.ResolveEndpoint(region, ec2.EndpointResolverOptions{})
	c.Assert(err, jc.ErrorIsNil)

	base := corebase.MustParseBaseFromString("ubuntu@22.04")
	err = s.makeLocalMetadata("1234", region, base, ep.URL, stream)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ValidateImageMetadataSuite) assertEc2LocalMetadataUsingEnvironment(c *gc.C, stream string) {
	s.setupEc2LocalMetadata(c, "us-east-1", stream)
	ctx, err := runValidateImageMetadata(c, s.store, "-c", "ec2-controller", "-d", s.metadataDir, "--stream", stream)
	c.Assert(err, jc.ErrorIsNil)
	stdout := cmdtesting.Stdout(ctx)
	stderr := cmdtesting.Stderr(ctx)
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
	_, err := runValidateImageMetadata(c, s.store, "-c", "ec2-controller", "-d", s.metadataDir)
	c.Assert(err, gc.ErrorMatches, `detecting credentials.*not found`)
}

func (s *ValidateImageMetadataSuite) TestEc2LocalMetadataWithManualParams(c *gc.C) {
	s.setupEc2LocalMetadata(c, "us-west-1", "")
	ctx, err := runValidateImageMetadata(c, s.store,
		"-p", "ec2", "--base", "ubuntu@22.04", "-r", "us-west-1",
		"-u", "https://ec2.us-west-1.amazonaws.com", "-d", s.metadataDir,
	)
	c.Assert(err, jc.ErrorIsNil)
	errOut := cmdtesting.Stdout(ctx)
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(
		strippedOut, gc.Matches,
		`ImageIds:.*"1234".*Region:.*us-west-1.*Resolve Metadata:.*source: local metadata directory.*`)
}

func (s *ValidateImageMetadataSuite) TestEc2LocalMetadataNoMatch(c *gc.C) {
	s.setupEc2LocalMetadata(c, "us-east-1", "")
	_, err := runValidateImageMetadata(c, s.store,
		"-p", "ec2", "--base", "ubuntu@13.04", "-r", "us-west-1",
		"-u", "https://ec2.us-west-1.amazonaws.com", "-d", s.metadataDir,
	)
	c.Check(err, gc.ErrorMatches, "(.|\n)*Resolve Metadata:(.|\n)*")
	_, err = runValidateImageMetadata(c, s.store,
		"-p", "ec2", "--base", "ubuntu@22.04", "-r", "region",
		"-u", "https://ec2.region.amazonaws.com", "-d", s.metadataDir,
	)
	c.Assert(err, gc.NotNil)
	msg := strings.ReplaceAll(err.Error(), "\n", "")
	c.Check(msg, gc.Matches, `index file has no data for cloud.*`)
}

func (s *ValidateImageMetadataSuite) TestOpenstackLocalMetadataWithManualParams(c *gc.C) {
	base := corebase.MustParseBaseFromString("ubuntu@13.04")
	err := s.makeLocalMetadata("1234", "region-2", base, "some-auth-url", "")
	c.Assert(err, jc.ErrorIsNil)
	ctx, err := runValidateImageMetadata(c, s.store,
		"-p", "openstack", "--base", "ubuntu@13.04", "-r", "region-2",
		"-u", "some-auth-url", "-d", s.metadataDir,
	)
	c.Assert(err, jc.ErrorIsNil)
	errOut := cmdtesting.Stdout(ctx)
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(
		strippedOut, gc.Matches,
		`ImageIds:.*"1234".*Region:.*region-2.*Resolve Metadata:.*source: local metadata directory.*`)
}

func (s *ValidateImageMetadataSuite) TestOpenstackLocalMetadataNoMatch(c *gc.C) {
	base := corebase.MustParseBaseFromString("ubuntu@13.04")
	err := s.makeLocalMetadata("1234", "region-2", base, "some-auth-url", "")
	c.Assert(err, jc.ErrorIsNil)
	_, err = runValidateImageMetadata(c, s.store,
		"-p", "openstack", "--base", "ubuntu@22.04", "-r", "region-2",
		"-u", "some-auth-url", "-d", s.metadataDir,
	)
	c.Check(err, gc.ErrorMatches, "(.|\n)*Resolve Metadata:(.|\n)*")
	_, err = runValidateImageMetadata(c, s.store,
		"-p", "openstack", "--base", "ubuntu@13.04", "-r", "region-3",
		"-u", "some-auth-url", "-d", s.metadataDir,
	)
	c.Check(err, gc.ErrorMatches, "(.|\n)*Resolve Metadata:(.|\n)*")
}

func (s *ValidateImageMetadataSuite) TestImagesDataSourceHasKey(c *gc.C) {
	ss := simplestreams.NewSimpleStreams(sstestings.TestDataSourceFactory())
	ds := imagesDataSources(ss, "test.me")
	// This data source does not require to contain signed data.
	// However, it may still contain it.
	// Since we will always try to read signed data first,
	// we want to be able to try to read this signed data
	// with a user provided public key. For this test, none is provided.
	// Bugs #1542127, #1542131
	c.Assert(ds[0].PublicSigningKey(), gc.Equals, "")
}
