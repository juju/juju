// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"strings"

	"launchpad.net/goamz/aws"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs/filestorage"
	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/simplestreams"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/testbase"
)

type ValidateImageMetadataSuite struct {
	testbase.LoggingSuite
	home        *coretesting.FakeHome
	metadataDir string
}

var _ = gc.Suite(&ValidateImageMetadataSuite{})

func runValidateImageMetadata(c *gc.C, args ...string) error {
	_, err := coretesting.RunCommand(c, &ValidateImageMetadataCommand{}, args)
	return err
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
		err := coretesting.InitCommand(&ValidateImageMetadataCommand{}, t.args)
		c.Check(err, gc.ErrorMatches, t.err)
	}
}

func (s *ValidateImageMetadataSuite) TestInvalidProviderError(c *gc.C) {
	err := runValidateImageMetadata(c, "-p", "foo", "-s", "series", "-r", "region", "-d", "dir")
	c.Check(err, gc.ErrorMatches, `no registered provider for "foo"`)
}

func (s *ValidateImageMetadataSuite) TestUnsupportedProviderError(c *gc.C) {
	err := runValidateImageMetadata(c, "-p", "local", "-s", "series", "-r", "region", "-d", "dir")
	c.Check(err, gc.ErrorMatches, `local provider does not support image metadata validation`)
}

func (s *ValidateImageMetadataSuite) makeLocalMetadata(c *gc.C, id, region, series, endpoint string) error {
	im := &imagemetadata.ImageMetadata{
		Id:   id,
		Arch: "amd64",
	}
	cloudSpec := simplestreams.CloudSpec{
		Region:   region,
		Endpoint: endpoint,
	}
	targetStorage, err := filestorage.NewFileStorageWriter(s.metadataDir, filestorage.UseDefaultTmpDir)
	if err != nil {
		return err
	}
	err = imagemetadata.MergeAndWriteMetadata(series, []*imagemetadata.ImageMetadata{im}, &cloudSpec, targetStorage)
	if err != nil {
		return err
	}
	return nil
}

const metadataTestEnvConfig = `
environments:
    ec2:
        type: ec2
        default-series: precise
        region: us-east-1

    azure:
        type: azure
        default-series: raring
        location: US West
        management-subscription-id: foo
        storage-account-name: bar
        management-certificate-path: /home/me/azure.pem
`

func (s *ValidateImageMetadataSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.metadataDir = c.MkDir()
	s.home = coretesting.MakeFakeHome(c, metadataTestEnvConfig)
	s.PatchEnvironment("AWS_ACCESS_KEY_ID", "access")
	s.PatchEnvironment("AWS_SECRET_ACCESS_KEY", "secret")
}

func (s *ValidateImageMetadataSuite) TearDownTest(c *gc.C) {
	s.home.Restore()
	s.LoggingSuite.TearDownTest(c)
}

func (s *ValidateImageMetadataSuite) setupEc2LocalMetadata(c *gc.C, region string) {
	ec2Region, ok := aws.Regions[region]
	if !ok {
		c.Fatalf("unknown ec2 region %q", region)
	}
	endpoint := ec2Region.EC2Endpoint
	s.makeLocalMetadata(c, "1234", region, "precise", endpoint)
}

func (s *ValidateImageMetadataSuite) TestEc2LocalMetadataUsingEnvironment(c *gc.C) {
	s.setupEc2LocalMetadata(c, "us-east-1")
	ctx := coretesting.Context(c)
	code := cmd.Main(
		&ValidateImageMetadataCommand{}, ctx, []string{"-e", "ec2", "-d", s.metadataDir},
	)
	c.Assert(code, gc.Equals, 0)
	errOut := ctx.Stdout.(*bytes.Buffer).String()
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, gc.Matches, `matching image ids for region "us-east-1":.*`)
}

func (s *ValidateImageMetadataSuite) TestEc2LocalMetadataUsingIncompleteEnvironment(c *gc.C) {
	testbase.PatchEnvironment("AWS_ACCESS_KEY_ID", "")
	testbase.PatchEnvironment("AWS_SECRET_ACCESS_KEY", "")
	testbase.PatchEnvironment("EC2_ACCESS_KEY", "")
	testbase.PatchEnvironment("EC2_SECRET_KEY", "")
	s.setupEc2LocalMetadata(c, "us-east-1")
	ctx := coretesting.Context(c)
	code := cmd.Main(
		&ValidateImageMetadataCommand{}, ctx, []string{"-e", "ec2", "-d", s.metadataDir},
	)
	c.Assert(code, gc.Equals, 1)
	errOut := ctx.Stderr.(*bytes.Buffer).String()
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, gc.Matches, `error: environment has no access-key or secret-key`)
}

func (s *ValidateImageMetadataSuite) TestEc2LocalMetadataWithManualParams(c *gc.C) {
	s.setupEc2LocalMetadata(c, "us-west-1")
	ctx := coretesting.Context(c)
	code := cmd.Main(
		&ValidateImageMetadataCommand{}, ctx, []string{
			"-p", "ec2", "-s", "precise", "-r", "us-west-1",
			"-u", "https://ec2.us-west-1.amazonaws.com", "-d", s.metadataDir},
	)
	c.Assert(code, gc.Equals, 0)
	errOut := ctx.Stdout.(*bytes.Buffer).String()
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, gc.Matches, `matching image ids for region "us-west-1":.*`)
}

func (s *ValidateImageMetadataSuite) TestEc2LocalMetadataNoMatch(c *gc.C) {
	s.setupEc2LocalMetadata(c, "us-east-1")
	ctx := coretesting.Context(c)
	code := cmd.Main(
		&ValidateImageMetadataCommand{}, ctx, []string{
			"-p", "ec2", "-s", "raring", "-r", "us-west-1",
			"-u", "https://ec2.us-west-1.amazonaws.com", "-d", s.metadataDir},
	)
	c.Assert(code, gc.Equals, 1)
	code = cmd.Main(
		&ValidateImageMetadataCommand{}, ctx, []string{
			"-p", "ec2", "-s", "precise", "-r", "region",
			"-u", "https://ec2.region.amazonaws.com", "-d", s.metadataDir},
	)
	c.Assert(code, gc.Equals, 1)
}

func (s *ValidateImageMetadataSuite) TestOpenstackLocalMetadataWithManualParams(c *gc.C) {
	s.makeLocalMetadata(c, "1234", "region-2", "raring", "some-auth-url")
	ctx := coretesting.Context(c)
	code := cmd.Main(
		&ValidateImageMetadataCommand{}, ctx, []string{
			"-p", "openstack", "-s", "raring", "-r", "region-2",
			"-u", "some-auth-url", "-d", s.metadataDir},
	)
	c.Assert(code, gc.Equals, 0)
	errOut := ctx.Stdout.(*bytes.Buffer).String()
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, gc.Matches, `matching image ids for region "region-2":.*`)
}

func (s *ValidateImageMetadataSuite) TestOpenstackLocalMetadataNoMatch(c *gc.C) {
	s.makeLocalMetadata(c, "1234", "region-2", "raring", "some-auth-url")
	ctx := coretesting.Context(c)
	code := cmd.Main(
		&ValidateImageMetadataCommand{}, ctx, []string{
			"-p", "openstack", "-s", "precise", "-r", "region-2",
			"-u", "some-auth-url", "-d", s.metadataDir},
	)
	c.Assert(code, gc.Equals, 1)
	code = cmd.Main(
		&ValidateImageMetadataCommand{}, ctx, []string{
			"-p", "openstack", "-s", "raring", "-r", "region-3",
			"-u", "some-auth-url", "-d", s.metadataDir},
	)
	c.Assert(code, gc.Equals, 1)
}
