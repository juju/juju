// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"strings"

	"launchpad.net/goamz/aws"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/juju/osenv"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/version"
)

type ValidateToolsMetadataSuite struct {
	testbase.LoggingSuite
	home *coretesting.FakeHome
}

var _ = gc.Suite(&ValidateToolsMetadataSuite{})

func runValidateToolsMetadata(c *gc.C, args ...string) error {
	_, err := coretesting.RunCommand(c, &ValidateToolsMetadataCommand{}, args)
	return err
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
		args: []string{"-s", "series", "-r", "region", "-m", "x"},
		err:  `invalid major version number x: .*`,
	}, {
		args: []string{"-s", "series", "-r", "region", "-m", "2.x"},
		err:  `invalid minor version number x: .*`,
	}, {
		args: []string{"-s", "series", "-r", "region", "-m", "2.2.1"},
		err:  `invalid major.minor version number 2.2.1`,
	},
}

func (s *ValidateToolsMetadataSuite) TestInitErrors(c *gc.C) {
	for i, t := range validateInitToolsErrorTests {
		c.Logf("test %d", i)
		err := coretesting.InitCommand(&ValidateToolsMetadataCommand{}, t.args)
		c.Check(err, gc.ErrorMatches, t.err)
	}
}

func (s *ValidateToolsMetadataSuite) TestInvalidProviderError(c *gc.C) {
	err := runValidateToolsMetadata(c, "-p", "foo", "-s", "series", "-r", "region", "-d", "dir")
	c.Check(err, gc.ErrorMatches, `no registered provider for "foo"`)
}

func (s *ValidateToolsMetadataSuite) TestUnsupportedProviderError(c *gc.C) {
	err := runValidateToolsMetadata(c, "-p", "local", "-s", "series", "-r", "region", "-d", "dir")
	c.Check(err, gc.ErrorMatches, `local provider does not support tools metadata validation`)
}

func (s *ValidateToolsMetadataSuite) makeLocalMetadata(c *gc.C, version, region, series, endpoint string) error {
	tm := tools.ToolsMetadata{
		Version: version,
		Arch:    "amd64",
		Release: series,
	}
	cloudSpec := simplestreams.CloudSpec{
		Region:   region,
		Endpoint: endpoint,
	}
	_, err := tools.MakeBoilerplate(&tm, &cloudSpec, false)
	if err != nil {
		return err
	}
	return nil
}

func (s *ValidateToolsMetadataSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.home = coretesting.MakeFakeHome(c, metadataTestEnvConfig)
	restore := testbase.PatchEnvironment("AWS_ACCESS_KEY_ID", "access")
	s.AddCleanup(func(*gc.C) { restore() })
	restore = testbase.PatchEnvironment("AWS_SECRET_ACCESS_KEY", "secret")
	s.AddCleanup(func(*gc.C) { restore() })
}

func (s *ValidateToolsMetadataSuite) TearDownTest(c *gc.C) {
	s.home.Restore()
	s.LoggingSuite.TearDownTest(c)
}

func (s *ValidateToolsMetadataSuite) setupEc2LocalMetadata(c *gc.C, region string) {
	ec2Region, ok := aws.Regions[region]
	if !ok {
		c.Fatalf("unknown ec2 region %q", region)
	}
	endpoint := ec2Region.EC2Endpoint
	s.makeLocalMetadata(c, "1.11.4", region, "precise", endpoint)
}

func (s *ValidateToolsMetadataSuite) TestEc2LocalMetadataUsingEnvironment(c *gc.C) {
	s.setupEc2LocalMetadata(c, "us-east-1")
	ctx := coretesting.Context(c)
	metadataDir := osenv.JujuHomePath("")
	code := cmd.Main(
		&ValidateToolsMetadataCommand{}, ctx, []string{"-e", "ec2", "-j", "1.11.4", "-d", metadataDir},
	)
	c.Assert(code, gc.Equals, 0)
	errOut := ctx.Stdout.(*bytes.Buffer).String()
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, gc.Matches, `matching tools versions:.*`)
}

func (s *ValidateToolsMetadataSuite) TestEc2LocalMetadataUsingIncompleteEnvironment(c *gc.C) {
	testbase.PatchEnvironment("AWS_ACCESS_KEY_ID", "")
	testbase.PatchEnvironment("AWS_SECRET_ACCESS_KEY", "")
	s.setupEc2LocalMetadata(c, "us-east-1")
	ctx := coretesting.Context(c)
	code := cmd.Main(
		&ValidateToolsMetadataCommand{}, ctx, []string{"-e", "ec2", "-j", "1.11.4"},
	)
	c.Assert(code, gc.Equals, 1)
	errOut := ctx.Stderr.(*bytes.Buffer).String()
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, gc.Matches, `error: environment has no access-key or secret-key`)
}

func (s *ValidateToolsMetadataSuite) TestEc2LocalMetadataWithManualParams(c *gc.C) {
	s.setupEc2LocalMetadata(c, "us-west-1")
	ctx := coretesting.Context(c)
	metadataDir := osenv.JujuHomePath("")
	code := cmd.Main(
		&ValidateToolsMetadataCommand{}, ctx, []string{
			"-p", "ec2", "-s", "precise", "-r", "us-west-1", "-j", "1.11.4",
			"-u", "https://ec2.us-west-1.amazonaws.com", "-d", metadataDir},
	)
	c.Assert(code, gc.Equals, 0)
	errOut := ctx.Stdout.(*bytes.Buffer).String()
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, gc.Matches, `matching tools versions:.*`)
}

func (s *ValidateToolsMetadataSuite) TestEc2LocalMetadataNoMatch(c *gc.C) {
	s.setupEc2LocalMetadata(c, "us-east-1")
	ctx := coretesting.Context(c)
	metadataDir := osenv.JujuHomePath("")
	code := cmd.Main(
		&ValidateToolsMetadataCommand{}, ctx, []string{
			"-p", "ec2", "-s", "raring", "-r", "us-west-1",
			"-u", "https://ec2.us-west-1.amazonaws.com", "-d", metadataDir},
	)
	c.Assert(code, gc.Equals, 1)
	code = cmd.Main(
		&ValidateToolsMetadataCommand{}, ctx, []string{
			"-p", "ec2", "-s", "precise", "-r", "region",
			"-u", "https://ec2.region.amazonaws.com", "-d", metadataDir},
	)
	c.Assert(code, gc.Equals, 1)
}

func (s *ValidateToolsMetadataSuite) TestOpenstackLocalMetadataWithManualParams(c *gc.C) {
	s.makeLocalMetadata(c, "1.11.4", "region-2", "raring", "some-auth-url")
	ctx := coretesting.Context(c)
	metadataDir := osenv.JujuHomePath("")
	code := cmd.Main(
		&ValidateToolsMetadataCommand{}, ctx, []string{
			"-p", "openstack", "-s", "raring", "-r", "region-2", "-j", "1.11.4",
			"-u", "some-auth-url", "-d", metadataDir},
	)
	c.Assert(code, gc.Equals, 0)
	errOut := ctx.Stdout.(*bytes.Buffer).String()
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, gc.Matches, `matching tools versions:.*`)
}

func (s *ValidateToolsMetadataSuite) TestOpenstackLocalMetadataNoMatch(c *gc.C) {
	s.makeLocalMetadata(c, "1.11.4", "region-2", "raring", "some-auth-url")
	ctx := coretesting.Context(c)
	metadataDir := osenv.JujuHomePath("")
	code := cmd.Main(
		&ValidateToolsMetadataCommand{}, ctx, []string{
			"-p", "openstack", "-s", "precise", "-r", "region-2",
			"-u", "some-auth-url", "-d", metadataDir},
	)
	c.Assert(code, gc.Equals, 1)
	code = cmd.Main(
		&ValidateToolsMetadataCommand{}, ctx, []string{
			"-p", "openstack", "-s", "raring", "-r", "region-3",
			"-u", "some-auth-url", "-d", metadataDir},
	)
	c.Assert(code, gc.Equals, 1)
}

func (s *ValidateToolsMetadataSuite) TestDefaultVersion(c *gc.C) {
	s.makeLocalMetadata(c, version.Current.Number.String(), "region-2", "raring", "some-auth-url")
	ctx := coretesting.Context(c)
	metadataDir := osenv.JujuHomePath("")
	code := cmd.Main(
		&ValidateToolsMetadataCommand{}, ctx, []string{
			"-p", "openstack", "-s", "raring", "-r", "region-2",
			"-u", "some-auth-url", "-d", metadataDir},
	)
	c.Assert(code, gc.Equals, 0)
	errOut := ctx.Stdout.(*bytes.Buffer).String()
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, gc.Matches, `matching tools versions:.*`)
}

func (s *ValidateToolsMetadataSuite) TestMajorVersionMatch(c *gc.C) {
	s.makeLocalMetadata(c, "1.11.4", "region-2", "raring", "some-auth-url")
	ctx := coretesting.Context(c)
	metadataDir := osenv.JujuHomePath("")
	code := cmd.Main(
		&ValidateToolsMetadataCommand{}, ctx, []string{
			"-p", "openstack", "-s", "raring", "-r", "region-2",
			"-u", "some-auth-url", "-d", metadataDir, "-m", "1"},
	)
	c.Assert(code, gc.Equals, 0)
	errOut := ctx.Stdout.(*bytes.Buffer).String()
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, gc.Matches, `matching tools versions:.*`)
}

func (s *ValidateToolsMetadataSuite) TestMajorMinorVersionMatch(c *gc.C) {
	s.makeLocalMetadata(c, "1.12.1", "region-2", "raring", "some-auth-url")
	ctx := coretesting.Context(c)
	metadataDir := osenv.JujuHomePath("")
	code := cmd.Main(
		&ValidateToolsMetadataCommand{}, ctx, []string{
			"-p", "openstack", "-s", "raring", "-r", "region-2",
			"-u", "some-auth-url", "-d", metadataDir, "-m", "1.12"},
	)
	c.Assert(code, gc.Equals, 0)
	errOut := ctx.Stdout.(*bytes.Buffer).String()
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, gc.Matches, `matching tools versions:.*`)
}

func (s *ValidateToolsMetadataSuite) TestJustDirectory(c *gc.C) {
	s.makeLocalMetadata(c, version.Current.Number.String(), "region-2", "raring", "some-auth-url")
	ctx := coretesting.Context(c)
	metadataDir := osenv.JujuHomePath("")
	code := cmd.Main(
		&ValidateToolsMetadataCommand{}, ctx, []string{"-s", "raring", "-d", metadataDir},
	)
	c.Assert(code, gc.Equals, 0)
	errOut := ctx.Stdout.(*bytes.Buffer).String()
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, gc.Matches, `matching tools versions:.*`)
}
