// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"strings"

	"launchpad.net/goamz/aws"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/juju/testing"
	coretesting "launchpad.net/juju-core/testing"
)

type ValidateMetadataSuite struct {
	testing.RepoSuite
	home *coretesting.FakeHome
}

var _ = gc.Suite(&ValidateMetadataSuite{})

func runValidateMetadata(c *gc.C, args ...string) error {
	_, err := coretesting.RunCommand(c, &ValidateMetadataCommand{}, args)
	return err
}

var validateInitErrorTests = []struct {
	args []string
	err  string
}{
	{
		args: []string{"-p", "ec2", "-r", "region"},
		err:  `series required if provider type is specified`,
	}, {
		args: []string{"-p", "ec2", "-s", "series"},
		err:  `region required if provider type is specified`,
	},
}

func (s *ValidateMetadataSuite) TestInitErrors(c *gc.C) {
	for i, t := range validateInitErrorTests {
		c.Logf("test %d", i)
		err := coretesting.InitCommand(&ValidateMetadataCommand{}, t.args)
		c.Check(err, gc.ErrorMatches, t.err)
	}
}

func (s *ValidateMetadataSuite) TestInvalidProviderError(c *gc.C) {
	err := runValidateMetadata(c, "-p", "foo", "-s", "series", "-r", "region")
	c.Check(err, gc.ErrorMatches, `no registered provider for "foo"`)
}

func (s *ValidateMetadataSuite) TestUnsupportedProviderError(c *gc.C) {
	err := runValidateMetadata(c, "-p", "local", "-s", "series", "-r", "region")
	c.Check(err, gc.ErrorMatches, `local provider does not support image metadata validation`)
}

func (s *ValidateMetadataSuite) makeLocalMetadata(c *gc.C, id, region, series, endpoint string) error {
	im := imagemetadata.ImageMetadata{
		Id:   id,
		Arch: "amd64",
	}
	cloudSpec := imagemetadata.CloudSpec{
		Region:   region,
		Endpoint: endpoint,
	}
	_, err := imagemetadata.MakeBoilerplate("", series, &im, &cloudSpec, false)
	if err != nil {
		return err
	}
	return nil
}

const metadataTestEnvConfig = `
environments:
    ec2:
        type: ec2
        control-bucket: foo
        default-series: precise
        region: us-east-1
`

func (s *ValidateMetadataSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
	s.home = coretesting.MakeFakeHome(c, metadataTestEnvConfig)
}

func (s *ValidateMetadataSuite) TearDownTest(c *gc.C) {
	s.home.Restore()
	s.RepoSuite.TearDownTest(c)
}

func (s *ValidateMetadataSuite) setupEc2LocalMetadata(c *gc.C, region string) {
	ec2Region, ok := aws.Regions[region]
	if !ok {
		c.Fatalf("unknown ec2 region %q")
	}
	endpoint := ec2Region.EC2Endpoint
	s.makeLocalMetadata(c, "1234", region, "precise", endpoint)
}

func (s *ValidateMetadataSuite) TestEc2LocalMetadataUsingEnvironment(c *gc.C) {
	s.setupEc2LocalMetadata(c, "us-east-1")
	ctx := coretesting.Context(c)
	metadataDir := config.JujuHomePath("")
	code := cmd.Main(
		&ValidateMetadataCommand{}, ctx, []string{"-e", "ec2", "-d", metadataDir},
	)
	c.Assert(code, gc.Equals, 0)
	errOut := ctx.Stdout.(*bytes.Buffer).String()
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, gc.Matches, `matching image ids for region "us-east-1":.*`)
}

func (s *ValidateMetadataSuite) TestEc2LocalMetadataWithManualParams(c *gc.C) {
	s.setupEc2LocalMetadata(c, "us-west-1")
	ctx := coretesting.Context(c)
	metadataDir := config.JujuHomePath("")
	code := cmd.Main(
		&ValidateMetadataCommand{}, ctx, []string{
			"-p", "ec2", "-s", "precise", "-r", "us-west-1",
			"-u", "https://ec2.us-west-1.amazonaws.com", "-d", metadataDir},
	)
	c.Assert(code, gc.Equals, 0)
	errOut := ctx.Stdout.(*bytes.Buffer).String()
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, gc.Matches, `matching image ids for region "us-west-1":.*`)
}

func (s *ValidateMetadataSuite) TestEc2LocalMetadataNoMatch(c *gc.C) {
	s.setupEc2LocalMetadata(c, "us-east-1")
	ctx := coretesting.Context(c)
	metadataDir := config.JujuHomePath("")
	code := cmd.Main(
		&ValidateMetadataCommand{}, ctx, []string{
			"-p", "ec2", "-s", "raring", "-r", "us-west-1",
			"-u", "https://ec2.us-west-1.amazonaws.com", "-d", metadataDir},
	)
	c.Assert(code, gc.Equals, 1)
	code = cmd.Main(
		&ValidateMetadataCommand{}, ctx, []string{
			"-p", "ec2", "-s", "precise", "-r", "region",
			"-u", "https://ec2.region.amazonaws.com", "-d", metadataDir},
	)
	c.Assert(code, gc.Equals, 1)
}

func (s *ValidateMetadataSuite) TestOpenstackLocalMetadataWithManualParams(c *gc.C) {
	s.makeLocalMetadata(c, "1234", "region-2", "raring", "some-auth-url")
	ctx := coretesting.Context(c)
	metadataDir := config.JujuHomePath("")
	code := cmd.Main(
		&ValidateMetadataCommand{}, ctx, []string{
			"-p", "openstack", "-s", "raring", "-r", "region-2",
			"-u", "some-auth-url", "-d", metadataDir},
	)
	c.Assert(code, gc.Equals, 0)
	errOut := ctx.Stdout.(*bytes.Buffer).String()
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, gc.Matches, `matching image ids for region "region-2":.*`)
}

func (s *ValidateMetadataSuite) TestOpenstackLocalMetadataNoMatch(c *gc.C) {
	s.makeLocalMetadata(c, "1234", "region-2", "raring", "some-auth-url")
	ctx := coretesting.Context(c)
	metadataDir := config.JujuHomePath("")
	code := cmd.Main(
		&ValidateMetadataCommand{}, ctx, []string{
			"-p", "openstack", "-s", "precise", "-r", "region-2",
			"-u", "some-auth-url", "-d", metadataDir},
	)
	c.Assert(code, gc.Equals, 1)
	code = cmd.Main(
		&ValidateMetadataCommand{}, ctx, []string{
			"-p", "openstack", "-s", "raring", "-r", "region-3",
			"-u", "some-auth-url", "-d", metadataDir},
	)
	c.Assert(code, gc.Equals, 1)
}
