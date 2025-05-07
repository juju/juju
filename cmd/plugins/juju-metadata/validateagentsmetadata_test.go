// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/arch"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
)

type ValidateToolsMetadataSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	metadataDir string
	store       *jujuclient.MemStore
}

var _ = tc.Suite(&ValidateToolsMetadataSuite{})

func runValidateAgentsMetadata(c *tc.C, store jujuclient.ClientStore, args ...string) (*cmd.Context, error) {
	cmd := &validateAgentsMetadataCommand{}
	cmd.SetClientStore(store)
	return cmdtesting.RunCommand(c, modelcmd.WrapController(cmd), args...)
}

var validateInitToolsErrorTests = []struct {
	args []string
	err  string
}{
	{
		args: []string{"-p", "ec2", "-t", "linux", "-d", "dir"},
		err:  `region required if provider type is specified`,
	}, {
		args: []string{"-p", "ec2", "-t", "linux", "-r", "region"},
		err:  `metadata directory required if provider type is specified`,
	}, {
		args: []string{"-t", "series", "-r", "region", "--majorminor-version", "x"},
		err:  `invalid major version number x: .*`,
	}, {
		args: []string{"-t", "series", "-r", "region", "--majorminor-version", "2.x"},
		err:  `invalid minor version number x: .*`,
	}, {
		args: []string{"-t", "series", "-r", "region", "--majorminor-version", "2.2.1"},
		err:  `invalid major.minor version number 2.2.1`,
	},
}

func (s *ValidateToolsMetadataSuite) TestInitErrors(c *tc.C) {
	for i, t := range validateInitToolsErrorTests {
		c.Logf("test %d", i)
		cmd := &validateAgentsMetadataCommand{}
		cmd.SetClientStore(s.store)
		err := cmdtesting.InitCommand(modelcmd.WrapController(cmd), t.args)
		c.Check(err, tc.ErrorMatches, t.err)
	}
}

func (s *ValidateToolsMetadataSuite) TestInvalidProviderError(c *tc.C) {
	_, err := runValidateAgentsMetadata(c, s.store, "-p", "foo", "-t", "linux", "-r", "region", "-d", "dir")
	c.Check(err, tc.ErrorMatches, `no registered provider for "foo"`)
}

func (s *ValidateToolsMetadataSuite) TestUnsupportedProviderError(c *tc.C) {
	_, err := runValidateAgentsMetadata(c, s.store, "-p", "maas", "-t", "linux", "-r", "region", "-d", "dir")
	c.Check(err, tc.ErrorMatches, `maas provider does not support metadata validation for agents`)
}

func (s *ValidateToolsMetadataSuite) makeLocalMetadata(c *tc.C, stream, version, region, osType, endpoint string) error {
	tm := []*tools.ToolsMetadata{{
		Version: version,
		Arch:    arch.HostArch(),
		Release: osType,
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

func (s *ValidateToolsMetadataSuite) SetUpTest(c *tc.C) {
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

func (s *ValidateToolsMetadataSuite) setupEc2LocalMetadata(c *tc.C, region string) {
	resolver := ec2.NewDefaultEndpointResolver()
	ep, err := resolver.ResolveEndpoint(region, ec2.EndpointResolverOptions{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.makeLocalMetadata(c, "released", "1.11.4", region, "ubuntu", ep.URL)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ValidateToolsMetadataSuite) TestEc2LocalMetadataUsingEnvironment(c *tc.C) {
	s.setupEc2LocalMetadata(c, "us-east-1")
	ctx, err := runValidateAgentsMetadata(c, s.store, "-c", "ec2-controller", "-j", "1.11.4", "-d", s.metadataDir)
	c.Assert(err, jc.ErrorIsNil)
	errOut := cmdtesting.Stdout(ctx)
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Assert(strippedOut, tc.Matches, `Matching Agent Binary Versions:.*Resolve Metadata.*`)
}

func (s *ValidateToolsMetadataSuite) TestEc2LocalMetadataUsingIncompleteEnvironment(c *tc.C) {
	// We already unset the other fallbacks recognized by goamz in SetUpTest().
	s.PatchEnvironment("AWS_ACCESS_KEY_ID", "")
	s.PatchEnvironment("AWS_SECRET_ACCESS_KEY", "")
	s.setupEc2LocalMetadata(c, "us-east-1")
	_, err := runValidateAgentsMetadata(c, s.store, "-c", "ec2-controller", "-j", "1.11.4")
	c.Assert(err, tc.ErrorMatches, `detecting credentials.*not found`)
}

func (s *ValidateToolsMetadataSuite) TestEc2LocalMetadataWithManualParams(c *tc.C) {
	s.setupEc2LocalMetadata(c, "us-west-1")
	ctx, err := runValidateAgentsMetadata(c, s.store,
		"-p", "ec2", "-t", "ubuntu", "-r", "us-west-1", "-j", "1.11.4",
		"-u", "https://ec2.us-west-1.amazonaws.com", "-d", s.metadataDir,
	)
	c.Assert(err, jc.ErrorIsNil)
	errOut := cmdtesting.Stdout(ctx)
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, tc.Matches, `Matching Agent Binary Versions:.*Resolve Metadata.*`)
}

func (s *ValidateToolsMetadataSuite) TestEc2LocalMetadataNoMatch(c *tc.C) {
	s.setupEc2LocalMetadata(c, "us-east-1")
	_, err := runValidateAgentsMetadata(c, s.store,
		"-p", "ec2", "-t", "windows", "-r", "us-west-1",
		"-u", "https://ec2.us-west-1.amazonaws.com", "-d", s.metadataDir,
	)
	c.Assert(err, tc.ErrorMatches, "no matching agent binaries(.|\n)*Resolve Metadata(.|\n)*")
	_, err = runValidateAgentsMetadata(c, s.store,
		"-p", "ec2", "-t", "ubuntu", "-r", "region",
		"-u", "https://ec2.region.amazonaws.com", "-d", s.metadataDir,
	)
	c.Assert(err, tc.NotNil)
	msg := strings.ReplaceAll(err.Error(), "\n", "")
	c.Check(msg, tc.Matches, `no matching agent binaries found for constraint.*`)
}

func (s *ValidateToolsMetadataSuite) TestOpenstackLocalMetadataWithManualParams(c *tc.C) {
	s.makeLocalMetadata(c, "released", "1.11.4", "region-2", "windows", "some-auth-url")
	ctx, err := runValidateAgentsMetadata(c, s.store,
		"-p", "openstack", "-t", "windows", "-r", "region-2", "-j", "1.11.4",
		"-u", "some-auth-url", "-d", s.metadataDir,
	)
	c.Assert(err, jc.ErrorIsNil)
	errOut := cmdtesting.Stdout(ctx)
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, tc.Matches, `Matching Agent Binary Versions:.*Resolve Metadata.*`)
}

func (s *ValidateToolsMetadataSuite) TestOpenstackLocalMetadataNoMatch(c *tc.C) {
	s.makeLocalMetadata(c, "released", "1.11.4", "region-2", "windows", "some-auth-url")
	_, err := runValidateAgentsMetadata(c, s.store,
		"-p", "openstack", "-t", "ubuntu", "-r", "region-2",
		"-u", "some-auth-url", "-d", s.metadataDir,
	)
	c.Assert(err, tc.ErrorMatches, "no matching agent binaries(.|\n)*Resolve Metadata(.|\n)*")
	_, err = runValidateAgentsMetadata(c, s.store,
		"-p", "openstack", "-t", "windows", "-r", "region-3",
		"-u", "some-auth-url", "-d", s.metadataDir,
	)
	c.Assert(err, tc.ErrorMatches, "no matching agent binaries(.|\n)*Resolve Metadata(.|\n)*")
}

func (s *ValidateToolsMetadataSuite) TestDefaultVersion(c *tc.C) {
	s.makeLocalMetadata(c, "released", jujuversion.Current.String(), "region-2", "windows", "some-auth-url")
	ctx, err := runValidateAgentsMetadata(c, s.store,
		"-p", "openstack", "-t", "windows", "-r", "region-2",
		"-u", "some-auth-url", "-d", s.metadataDir,
	)
	c.Assert(err, jc.ErrorIsNil)
	errOut := cmdtesting.Stdout(ctx)
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, tc.Matches, `Matching Agent Binary Versions:.*Resolve Metadata.*`)
}

func (s *ValidateToolsMetadataSuite) TestStream(c *tc.C) {
	s.makeLocalMetadata(c, "proposed", jujuversion.Current.String(), "region-2", "windows", "some-auth-url")
	ctx, err := runValidateAgentsMetadata(c, s.store,
		"-p", "openstack", "-t", "windows", "-r", "region-2",
		"-u", "some-auth-url", "-d", s.metadataDir, "--stream", "proposed",
	)
	c.Assert(err, jc.ErrorIsNil)
	errOut := cmdtesting.Stdout(ctx)
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, tc.Matches, `Matching Agent Binary Versions:.*Resolve Metadata.*`)
}

func (s *ValidateToolsMetadataSuite) TestMajorVersionMatch(c *tc.C) {
	s.makeLocalMetadata(c, "released", "1.11.4", "region-2", "windows", "some-auth-url")
	ctx, err := runValidateAgentsMetadata(c, s.store,
		"-p", "openstack", "-t", "windows", "-r", "region-2",
		"-u", "some-auth-url", "-d", s.metadataDir, "--majorminor-version", "1",
	)
	c.Assert(err, jc.ErrorIsNil)
	errOut := cmdtesting.Stdout(ctx)
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, tc.Matches, `Matching Agent Binary Versions:.*Resolve Metadata.*`)
}

func (s *ValidateToolsMetadataSuite) TestMajorMinorVersionMatch(c *tc.C) {
	s.makeLocalMetadata(c, "released", "1.12.1", "region-2", "windows", "some-auth-url")
	ctx, err := runValidateAgentsMetadata(c, s.store,
		"-p", "openstack", "-t", "windows", "-r", "region-2",
		"-u", "some-auth-url", "-d", s.metadataDir, "--majorminor-version", "1.12",
	)
	c.Assert(err, jc.ErrorIsNil)
	errOut := cmdtesting.Stdout(ctx)
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, tc.Matches, `Matching Agent Binary Versions:.*Resolve Metadata.*`)
}

func (s *ValidateToolsMetadataSuite) TestJustDirectory(c *tc.C) {
	s.makeLocalMetadata(c, "released", jujuversion.Current.String(), "region-2", "windows", "some-auth-url")
	ctx, err := runValidateAgentsMetadata(c, s.store,
		"-t", "windows", "-d", s.metadataDir,
	)
	c.Assert(err, jc.ErrorIsNil)
	errOut := cmdtesting.Stdout(ctx)
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, tc.Matches, `Matching Agent Binary Versions:.*Resolve Metadata.*`)
}
