// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/juju/tc"

	"github.com/juju/juju/cmd/modelcmd"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	sstestings "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
)

type ValidateImageMetadataSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	metadataDir string
	store       *jujuclient.MemStore
}

func TestValidateImageMetadataSuite(t *testing.T) {
	tc.Run(t, &ValidateImageMetadataSuite{})
}
func runValidateImageMetadata(c *tc.C, store jujuclient.ClientStore, args ...string) (*cmd.Context, error) {
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

func (s *ValidateImageMetadataSuite) TestInitErrors(c *tc.C) {
	for i, t := range validateInitImageErrorTests {
		c.Logf("test %d", i)
		cmd := &validateImageMetadataCommand{}
		cmd.SetClientStore(jujuclienttesting.MinimalStore())
		err := cmdtesting.InitCommand(modelcmd.WrapController(cmd), t.args)
		c.Check(err, tc.ErrorMatches, t.err)
	}
}

func (s *ValidateImageMetadataSuite) TestInvalidProviderError(c *tc.C) {
	_, err := runValidateImageMetadata(c, s.store, "-p", "foo", "--base", "ubuntu@22.04", "-r", "region", "-d", "dir")
	c.Check(err, tc.ErrorMatches, `no registered provider for "foo"`)
}

func (s *ValidateImageMetadataSuite) TestUnsupportedProviderError(c *tc.C) {
	_, err := runValidateImageMetadata(c, s.store, "-p", "maas", "--base", "ubuntu@22.04", "-r", "region", "-d", "dir")
	c.Check(err, tc.ErrorMatches, `maas provider does not support image metadata validation`)
}

func (s *ValidateImageMetadataSuite) makeLocalMetadata(c *tc.C, id, region string, base corebase.Base, endpoint, stream string) error {
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
	err = imagemetadata.MergeAndWriteMetadata(c.Context(), ss, base, []*imagemetadata.ImageMetadata{im}, &cloudSpec, targetStorage)
	if err != nil {
		return err
	}
	return nil
}

func cacheTestEnvConfig(c *tc.C, store *jujuclient.MemStore) {
	ec2UUID := uuid.MustNewUUID().String()
	ec2Config, err := config.New(config.UseDefaults, map[string]interface{}{
		"name":            "ec2",
		"type":            "ec2",
		"default-base":    "ubuntu@22.04",
		"controller-uuid": coretesting.ControllerTag.Id(),
		"uuid":            ec2UUID,
	})
	c.Assert(err, tc.ErrorIsNil)
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

func (s *ValidateImageMetadataSuite) SetUpTest(c *tc.C) {
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

func (s *ValidateImageMetadataSuite) setupEc2LocalMetadata(c *tc.C, region, stream string) {
	resolver := ec2.NewDefaultEndpointResolver()
	ep, err := resolver.ResolveEndpoint(region, ec2.EndpointResolverOptions{})
	c.Assert(err, tc.ErrorIsNil)

	base := corebase.MustParseBaseFromString("ubuntu@22.04")
	err = s.makeLocalMetadata(c, "1234", region, base, ep.URL, stream)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ValidateImageMetadataSuite) assertEc2LocalMetadataUsingEnvironment(c *tc.C, stream string) {
	s.setupEc2LocalMetadata(c, "us-east-1", stream)
	ctx, err := runValidateImageMetadata(c, s.store, "-c", "ec2-controller", "-d", s.metadataDir, "--stream", stream)
	c.Assert(err, tc.ErrorIsNil)
	stdout := cmdtesting.Stdout(ctx)
	stderr := cmdtesting.Stderr(ctx)
	strippedOut := strings.Replace(stdout, "\n", "", -1)
	c.Check(strippedOut, tc.Matches,
		`ImageIds:.*"1234".*Region:.*us-east-1.*Resolve Metadata:.*source: local metadata directory.*`,
	)
	c.Check(stderr, tc.Matches, "")
}

func (s *ValidateImageMetadataSuite) TestEc2LocalMetadataUsingEnvironment(c *tc.C) {
	s.assertEc2LocalMetadataUsingEnvironment(c, "")
	s.assertEc2LocalMetadataUsingEnvironment(c, imagemetadata.ReleasedStream)
	s.assertEc2LocalMetadataUsingEnvironment(c, "daily")
}

func (s *ValidateImageMetadataSuite) TestEc2LocalMetadataUsingIncompleteEnvironment(c *tc.C) {
	s.PatchEnvironment("AWS_ACCESS_KEY_ID", "")
	s.PatchEnvironment("AWS_SECRET_ACCESS_KEY", "")
	s.PatchEnvironment("EC2_ACCESS_KEY", "")
	s.PatchEnvironment("EC2_SECRET_KEY", "")
	s.setupEc2LocalMetadata(c, "us-east-1", "")
	_, err := runValidateImageMetadata(c, s.store, "-c", "ec2-controller", "-d", s.metadataDir)
	c.Assert(err, tc.ErrorMatches, `detecting credentials.*not found`)
}

func (s *ValidateImageMetadataSuite) TestEc2LocalMetadataWithManualParams(c *tc.C) {
	s.setupEc2LocalMetadata(c, "us-west-1", "")
	ctx, err := runValidateImageMetadata(c, s.store,
		"-p", "ec2", "--base", "ubuntu@22.04", "-r", "us-west-1",
		"-u", "https://ec2.us-west-1.amazonaws.com", "-d", s.metadataDir,
	)
	c.Assert(err, tc.ErrorIsNil)
	errOut := cmdtesting.Stdout(ctx)
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(
		strippedOut, tc.Matches,
		`ImageIds:.*"1234".*Region:.*us-west-1.*Resolve Metadata:.*source: local metadata directory.*`)
}

func (s *ValidateImageMetadataSuite) TestEc2LocalMetadataNoMatch(c *tc.C) {
	s.setupEc2LocalMetadata(c, "us-east-1", "")
	_, err := runValidateImageMetadata(c, s.store,
		"-p", "ec2", "--base", "ubuntu@13.04", "-r", "us-west-1",
		"-u", "https://ec2.us-west-1.amazonaws.com", "-d", s.metadataDir,
	)
	c.Check(err, tc.ErrorMatches, "(.|\n)*Resolve Metadata:(.|\n)*")
	_, err = runValidateImageMetadata(c, s.store,
		"-p", "ec2", "--base", "ubuntu@22.04", "-r", "region",
		"-u", "https://ec2.region.amazonaws.com", "-d", s.metadataDir,
	)
	c.Assert(err, tc.NotNil)
	msg := strings.ReplaceAll(err.Error(), "\n", "")
	c.Check(msg, tc.Matches, `index file has no data for cloud.*`)
}

func (s *ValidateImageMetadataSuite) TestOpenstackLocalMetadataWithManualParams(c *tc.C) {
	base := corebase.MustParseBaseFromString("ubuntu@13.04")
	err := s.makeLocalMetadata(c, "1234", "region-2", base, "some-auth-url", "")
	c.Assert(err, tc.ErrorIsNil)
	ctx, err := runValidateImageMetadata(c, s.store,
		"-p", "openstack", "--base", "ubuntu@13.04", "-r", "region-2",
		"-u", "some-auth-url", "-d", s.metadataDir,
	)
	c.Assert(err, tc.ErrorIsNil)
	errOut := cmdtesting.Stdout(ctx)
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(
		strippedOut, tc.Matches,
		`ImageIds:.*"1234".*Region:.*region-2.*Resolve Metadata:.*source: local metadata directory.*`)
}

func (s *ValidateImageMetadataSuite) TestOpenstackLocalMetadataNoMatch(c *tc.C) {
	base := corebase.MustParseBaseFromString("ubuntu@13.04")
	err := s.makeLocalMetadata(c, "1234", "region-2", base, "some-auth-url", "")
	c.Assert(err, tc.ErrorIsNil)
	_, err = runValidateImageMetadata(c, s.store,
		"-p", "openstack", "--base", "ubuntu@22.04", "-r", "region-2",
		"-u", "some-auth-url", "-d", s.metadataDir,
	)
	c.Check(err, tc.ErrorMatches, "(.|\n)*Resolve Metadata:(.|\n)*")
	_, err = runValidateImageMetadata(c, s.store,
		"-p", "openstack", "--base", "ubuntu@13.04", "-r", "region-3",
		"-u", "some-auth-url", "-d", s.metadataDir,
	)
	c.Check(err, tc.ErrorMatches, "(.|\n)*Resolve Metadata:(.|\n)*")
}

func (s *ValidateImageMetadataSuite) TestImagesDataSourceHasKey(c *tc.C) {
	ss := simplestreams.NewSimpleStreams(sstestings.TestDataSourceFactory())
	ds := imagesDataSources(ss, "test.me")
	// This data source does not require to contain signed data.
	// However, it may still contain it.
	// Since we will always try to read signed data first,
	// we want to be able to try to read this signed data
	// with a user provided public key. For this test, none is provided.
	// Bugs #1542127, #1542131
	c.Assert(ds[0].PublicSigningKey(), tc.Equals, "")
}
