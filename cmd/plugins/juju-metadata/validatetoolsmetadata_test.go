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
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type ValidateToolsMetadataSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	metadataDir string
	store       *jujuclienttesting.MemStore
}

var _ = gc.Suite(&ValidateToolsMetadataSuite{})

func runValidateToolsMetadata(c *gc.C, store jujuclient.ClientStore, args ...string) (*cmd.Context, error) {
	cmd := &validateToolsMetadataCommand{}
	cmd.SetClientStore(store)
	return coretesting.RunCommand(c, modelcmd.Wrap(cmd), args...)
}

var validateInitToolsErrorTests = []struct {
	args []string
	err  string
}{
	{
		args: []string{"-p", "ec2", "-s", "series", "-d", "dir"},
		err:  `region required if provider type is specified`,
	}, {
		args: []string{"-p", "ec2", "-s", "series", "-r", "region"},
		err:  `metadata directory required if provider type is specified`,
	}, {
		args: []string{"-s", "series", "-r", "region", "--majorminor-version", "x"},
		err:  `invalid major version number x: .*`,
	}, {
		args: []string{"-s", "series", "-r", "region", "--majorminor-version", "2.x"},
		err:  `invalid minor version number x: .*`,
	}, {
		args: []string{"-s", "series", "-r", "region", "--majorminor-version", "2.2.1"},
		err:  `invalid major.minor version number 2.2.1`,
	},
}

func (s *ValidateToolsMetadataSuite) TestInitErrors(c *gc.C) {
	for i, t := range validateInitToolsErrorTests {
		c.Logf("test %d", i)
		cmd := &validateToolsMetadataCommand{}
		cmd.SetClientStore(s.store)
		err := coretesting.InitCommand(modelcmd.Wrap(cmd), t.args)
		c.Check(err, gc.ErrorMatches, t.err)
	}
}

func (s *ValidateToolsMetadataSuite) TestInvalidProviderError(c *gc.C) {
	_, err := runValidateToolsMetadata(c, s.store, "-p", "foo", "-s", "series", "-r", "region", "-d", "dir")
	c.Check(err, gc.ErrorMatches, `no registered provider for "foo"`)
}

func (s *ValidateToolsMetadataSuite) TestUnsupportedProviderError(c *gc.C) {
	_, err := runValidateToolsMetadata(c, s.store, "-p", "maas", "-s", "series", "-r", "region", "-d", "dir")
	c.Check(err, gc.ErrorMatches, `maas provider does not support tools metadata validation`)
}

func (s *ValidateToolsMetadataSuite) makeLocalMetadata(c *gc.C, stream, version, region, series, endpoint string) error {
	tm := []*tools.ToolsMetadata{{
		Version: version,
		Arch:    "amd64",
		Release: series,
	}}
	targetStorage, err := filestorage.NewFileStorageWriter(s.metadataDir)
	c.Assert(err, jc.ErrorIsNil)
	streamMetadata := map[string][]*tools.ToolsMetadata{
		stream: tm,
	}
	err = tools.WriteMetadata(targetStorage, streamMetadata, []string{stream}, false)
	if err != nil {
		return err
	}
	return nil
}

func (s *ValidateToolsMetadataSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	cacheTestEnvConfig(c)
	s.metadataDir = c.MkDir()

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

func (s *ValidateToolsMetadataSuite) setupEc2LocalMetadata(c *gc.C, region string) {
	ec2Region, ok := aws.Regions[region]
	if !ok {
		c.Fatalf("unknown ec2 region %q", region)
	}
	endpoint := ec2Region.EC2Endpoint
	s.makeLocalMetadata(c, "released", "1.11.4", region, "precise", endpoint)
}

func (s *ValidateToolsMetadataSuite) TestEc2LocalMetadataUsingEnvironment(c *gc.C) {
	s.setupEc2LocalMetadata(c, "us-east-1")
	ctx, err := runValidateToolsMetadata(c, s.store, "-m", "ec2", "-j", "1.11.4", "-d", s.metadataDir)
	c.Assert(err, jc.ErrorIsNil)
	errOut := coretesting.Stdout(ctx)
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Assert(strippedOut, gc.Matches, `Matching Tools Versions:.*Resolve Metadata.*`)
}

func (s *ValidateToolsMetadataSuite) TestEc2LocalMetadataUsingIncompleteEnvironment(c *gc.C) {
	// We already unset the other fallbacks recognized by goamz in SetUpTest().
	s.PatchEnvironment("AWS_ACCESS_KEY_ID", "")
	s.PatchEnvironment("AWS_SECRET_ACCESS_KEY", "")
	s.setupEc2LocalMetadata(c, "us-east-1")
	_, err := runValidateToolsMetadata(c, s.store, "-m", "ec2", "-j", "1.11.4")
	c.Assert(err, gc.ErrorMatches, "invalid EC2 provider config: model has no access-key or secret-key")
}

func (s *ValidateToolsMetadataSuite) TestEc2LocalMetadataWithManualParams(c *gc.C) {
	s.setupEc2LocalMetadata(c, "us-west-1")
	ctx, err := runValidateToolsMetadata(c, s.store,
		"-p", "ec2", "-s", "precise", "-r", "us-west-1", "-j", "1.11.4",
		"-u", "https://ec2.us-west-1.amazonaws.com", "-d", s.metadataDir,
	)
	c.Assert(err, jc.ErrorIsNil)
	errOut := coretesting.Stdout(ctx)
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, gc.Matches, `Matching Tools Versions:.*Resolve Metadata.*`)
}

func (s *ValidateToolsMetadataSuite) TestEc2LocalMetadataNoMatch(c *gc.C) {
	s.setupEc2LocalMetadata(c, "us-east-1")
	_, err := runValidateToolsMetadata(c, s.store,
		"-p", "ec2", "-s", "raring", "-r", "us-west-1",
		"-u", "https://ec2.us-west-1.amazonaws.com", "-d", s.metadataDir,
	)
	c.Assert(err, gc.ErrorMatches, "no matching tools(.|\n)*Resolve Metadata(.|\n)*")
	_, err = runValidateToolsMetadata(c, s.store,
		"-p", "ec2", "-s", "precise", "-r", "region",
		"-u", "https://ec2.region.amazonaws.com", "-d", s.metadataDir,
	)
	c.Assert(err, gc.ErrorMatches, `unknown region "region"`)
}

func (s *ValidateToolsMetadataSuite) TestOpenstackLocalMetadataWithManualParams(c *gc.C) {
	s.makeLocalMetadata(c, "released", "1.11.4", "region-2", "raring", "some-auth-url")
	ctx, err := runValidateToolsMetadata(c, s.store,
		"-p", "openstack", "-s", "raring", "-r", "region-2", "-j", "1.11.4",
		"-u", "some-auth-url", "-d", s.metadataDir,
	)
	c.Assert(err, jc.ErrorIsNil)
	errOut := coretesting.Stdout(ctx)
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, gc.Matches, `Matching Tools Versions:.*Resolve Metadata.*`)
}

func (s *ValidateToolsMetadataSuite) TestOpenstackLocalMetadataNoMatch(c *gc.C) {
	s.makeLocalMetadata(c, "released", "1.11.4", "region-2", "raring", "some-auth-url")
	_, err := runValidateToolsMetadata(c, s.store,
		"-p", "openstack", "-s", "precise", "-r", "region-2",
		"-u", "some-auth-url", "-d", s.metadataDir,
	)
	c.Assert(err, gc.ErrorMatches, "no matching tools(.|\n)*Resolve Metadata(.|\n)*")
	_, err = runValidateToolsMetadata(c, s.store,
		"-p", "openstack", "-s", "raring", "-r", "region-3",
		"-u", "some-auth-url", "-d", s.metadataDir,
	)
	c.Assert(err, gc.ErrorMatches, "no matching tools(.|\n)*Resolve Metadata(.|\n)*")
}

func (s *ValidateToolsMetadataSuite) TestDefaultVersion(c *gc.C) {
	s.makeLocalMetadata(c, "released", version.Current.String(), "region-2", "raring", "some-auth-url")
	ctx, err := runValidateToolsMetadata(c, s.store,
		"-p", "openstack", "-s", "raring", "-r", "region-2",
		"-u", "some-auth-url", "-d", s.metadataDir,
	)
	c.Assert(err, jc.ErrorIsNil)
	errOut := coretesting.Stdout(ctx)
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, gc.Matches, `Matching Tools Versions:.*Resolve Metadata.*`)
}

func (s *ValidateToolsMetadataSuite) TestStream(c *gc.C) {
	s.makeLocalMetadata(c, "proposed", version.Current.String(), "region-2", "raring", "some-auth-url")
	ctx, err := runValidateToolsMetadata(c, s.store,
		"-p", "openstack", "-s", "raring", "-r", "region-2",
		"-u", "some-auth-url", "-d", s.metadataDir, "--stream", "proposed",
	)
	c.Assert(err, jc.ErrorIsNil)
	errOut := coretesting.Stdout(ctx)
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, gc.Matches, `Matching Tools Versions:.*Resolve Metadata.*`)
}

func (s *ValidateToolsMetadataSuite) TestMajorVersionMatch(c *gc.C) {
	s.makeLocalMetadata(c, "released", "1.11.4", "region-2", "raring", "some-auth-url")
	ctx, err := runValidateToolsMetadata(c, s.store,
		"-p", "openstack", "-s", "raring", "-r", "region-2",
		"-u", "some-auth-url", "-d", s.metadataDir, "--majorminor-version", "1",
	)
	c.Assert(err, jc.ErrorIsNil)
	errOut := coretesting.Stdout(ctx)
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, gc.Matches, `Matching Tools Versions:.*Resolve Metadata.*`)
}

func (s *ValidateToolsMetadataSuite) TestMajorMinorVersionMatch(c *gc.C) {
	s.makeLocalMetadata(c, "released", "1.12.1", "region-2", "raring", "some-auth-url")
	ctx, err := runValidateToolsMetadata(c, s.store,
		"-p", "openstack", "-s", "raring", "-r", "region-2",
		"-u", "some-auth-url", "-d", s.metadataDir, "--majorminor-version", "1.12",
	)
	c.Assert(err, jc.ErrorIsNil)
	errOut := coretesting.Stdout(ctx)
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, gc.Matches, `Matching Tools Versions:.*Resolve Metadata.*`)
}

func (s *ValidateToolsMetadataSuite) TestJustDirectory(c *gc.C) {
	s.makeLocalMetadata(c, "released", version.Current.String(), "region-2", "raring", "some-auth-url")
	ctx, err := runValidateToolsMetadata(c, s.store,
		"-s", "raring", "-d", s.metadataDir,
	)
	c.Assert(err, jc.ErrorIsNil)
	errOut := coretesting.Stdout(ctx)
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, gc.Matches, `Matching Tools Versions:.*Resolve Metadata.*`)
}
