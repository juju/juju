// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"strings"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	"gopkg.in/amz.v2/aws"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/tools"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type ValidateToolsMetadataSuite struct {
	coretesting.FakeJujuHomeSuite
	metadataDir string
}

var _ = gc.Suite(&ValidateToolsMetadataSuite{})

func runValidateToolsMetadata(c *gc.C, args ...string) error {
	_, err := coretesting.RunCommand(c, envcmd.Wrap(&ValidateToolsMetadataCommand{}), args...)
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
		err := coretesting.InitCommand(envcmd.Wrap(&ValidateToolsMetadataCommand{}), t.args)
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
	s.FakeJujuHomeSuite.SetUpTest(c)
	coretesting.WriteEnvironments(c, metadataTestEnvConfig)
	s.metadataDir = c.MkDir()
	s.PatchEnvironment("AWS_ACCESS_KEY_ID", "access")
	s.PatchEnvironment("AWS_SECRET_ACCESS_KEY", "secret")
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
	ctx := coretesting.Context(c)
	code := cmd.Main(
		envcmd.Wrap(&ValidateToolsMetadataCommand{}), ctx, []string{"-e", "ec2", "-j", "1.11.4", "-d", s.metadataDir},
	)
	c.Assert(code, gc.Equals, 0)
	errOut := ctx.Stdout.(*bytes.Buffer).String()
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, gc.Matches, `Matching Tools Versions:.*Resolve Metadata.*`)
}

func (s *ValidateToolsMetadataSuite) TestEc2LocalMetadataUsingIncompleteEnvironment(c *gc.C) {
	s.PatchEnvironment("AWS_ACCESS_KEY_ID", "")
	s.PatchEnvironment("AWS_SECRET_ACCESS_KEY", "")
	s.setupEc2LocalMetadata(c, "us-east-1")
	ctx := coretesting.Context(c)
	code := cmd.Main(
		envcmd.Wrap(&ValidateToolsMetadataCommand{}), ctx, []string{"-e", "ec2", "-j", "1.11.4"},
	)
	c.Assert(code, gc.Equals, 1)
	errOut := ctx.Stderr.(*bytes.Buffer).String()
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, gc.Matches, `error: .*environment has no access-key or secret-key`)
}

func (s *ValidateToolsMetadataSuite) TestEc2LocalMetadataWithManualParams(c *gc.C) {
	s.setupEc2LocalMetadata(c, "us-west-1")
	ctx := coretesting.Context(c)
	code := cmd.Main(
		envcmd.Wrap(&ValidateToolsMetadataCommand{}), ctx, []string{
			"-p", "ec2", "-s", "precise", "-r", "us-west-1", "-j", "1.11.4",
			"-u", "https://ec2.us-west-1.amazonaws.com", "-d", s.metadataDir},
	)
	c.Assert(code, gc.Equals, 0)
	errOut := ctx.Stdout.(*bytes.Buffer).String()
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, gc.Matches, `Matching Tools Versions:.*Resolve Metadata.*`)
}

func (s *ValidateToolsMetadataSuite) TestEc2LocalMetadataNoMatch(c *gc.C) {
	s.setupEc2LocalMetadata(c, "us-east-1")
	ctx := coretesting.Context(c)
	code := cmd.Main(
		envcmd.Wrap(&ValidateToolsMetadataCommand{}), ctx, []string{
			"-p", "ec2", "-s", "raring", "-r", "us-west-1",
			"-u", "https://ec2.us-west-1.amazonaws.com", "-d", s.metadataDir},
	)
	c.Assert(code, gc.Equals, 1)
	code = cmd.Main(
		envcmd.Wrap(&ValidateToolsMetadataCommand{}), ctx, []string{
			"-p", "ec2", "-s", "precise", "-r", "region",
			"-u", "https://ec2.region.amazonaws.com", "-d", s.metadataDir},
	)
	c.Assert(code, gc.Equals, 1)
	errOut := ctx.Stderr.(*bytes.Buffer).String()
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, gc.Matches, `.*Resolve Metadata:.*`)
}

func (s *ValidateToolsMetadataSuite) TestOpenstackLocalMetadataWithManualParams(c *gc.C) {
	s.makeLocalMetadata(c, "released", "1.11.4", "region-2", "raring", "some-auth-url")
	ctx := coretesting.Context(c)
	code := cmd.Main(
		envcmd.Wrap(&ValidateToolsMetadataCommand{}), ctx, []string{
			"-p", "openstack", "-s", "raring", "-r", "region-2", "-j", "1.11.4",
			"-u", "some-auth-url", "-d", s.metadataDir},
	)
	c.Assert(code, gc.Equals, 0)
	errOut := ctx.Stdout.(*bytes.Buffer).String()
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, gc.Matches, `Matching Tools Versions:.*Resolve Metadata.*`)
}

func (s *ValidateToolsMetadataSuite) TestOpenstackLocalMetadataNoMatch(c *gc.C) {
	s.makeLocalMetadata(c, "released", "1.11.4", "region-2", "raring", "some-auth-url")
	ctx := coretesting.Context(c)
	code := cmd.Main(
		envcmd.Wrap(&ValidateToolsMetadataCommand{}), ctx, []string{
			"-p", "openstack", "-s", "precise", "-r", "region-2",
			"-u", "some-auth-url", "-d", s.metadataDir},
	)
	c.Assert(code, gc.Equals, 1)
	code = cmd.Main(
		envcmd.Wrap(&ValidateToolsMetadataCommand{}), ctx, []string{
			"-p", "openstack", "-s", "raring", "-r", "region-3",
			"-u", "some-auth-url", "-d", s.metadataDir},
	)
	c.Assert(code, gc.Equals, 1)
	errOut := ctx.Stderr.(*bytes.Buffer).String()
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, gc.Matches, `.*Resolve Metadata:.*`)
}

func (s *ValidateToolsMetadataSuite) TestDefaultVersion(c *gc.C) {
	s.makeLocalMetadata(c, "released", version.Current.Number.String(), "region-2", "raring", "some-auth-url")
	ctx := coretesting.Context(c)
	code := cmd.Main(
		envcmd.Wrap(&ValidateToolsMetadataCommand{}), ctx, []string{
			"-p", "openstack", "-s", "raring", "-r", "region-2",
			"-u", "some-auth-url", "-d", s.metadataDir},
	)
	c.Assert(code, gc.Equals, 0)
	errOut := ctx.Stdout.(*bytes.Buffer).String()
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, gc.Matches, `Matching Tools Versions:.*Resolve Metadata.*`)
}

func (s *ValidateToolsMetadataSuite) TestStream(c *gc.C) {
	s.makeLocalMetadata(c, "proposed", version.Current.Number.String(), "region-2", "raring", "some-auth-url")
	ctx := coretesting.Context(c)
	code := cmd.Main(
		envcmd.Wrap(&ValidateToolsMetadataCommand{}), ctx, []string{
			"-p", "openstack", "-s", "raring", "-r", "region-2",
			"-u", "some-auth-url", "-d", s.metadataDir, "--stream", "proposed"},
	)
	c.Assert(code, gc.Equals, 0)
	errOut := ctx.Stdout.(*bytes.Buffer).String()
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, gc.Matches, `Matching Tools Versions:.*Resolve Metadata.*`)
}

func (s *ValidateToolsMetadataSuite) TestMajorVersionMatch(c *gc.C) {
	s.makeLocalMetadata(c, "released", "1.11.4", "region-2", "raring", "some-auth-url")
	ctx := coretesting.Context(c)
	code := cmd.Main(
		envcmd.Wrap(&ValidateToolsMetadataCommand{}), ctx, []string{
			"-p", "openstack", "-s", "raring", "-r", "region-2",
			"-u", "some-auth-url", "-d", s.metadataDir, "-m", "1"},
	)
	c.Assert(code, gc.Equals, 0)
	errOut := ctx.Stdout.(*bytes.Buffer).String()
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, gc.Matches, `Matching Tools Versions:.*Resolve Metadata.*`)
}

func (s *ValidateToolsMetadataSuite) TestMajorMinorVersionMatch(c *gc.C) {
	s.makeLocalMetadata(c, "released", "1.12.1", "region-2", "raring", "some-auth-url")
	ctx := coretesting.Context(c)
	code := cmd.Main(
		envcmd.Wrap(&ValidateToolsMetadataCommand{}), ctx, []string{
			"-p", "openstack", "-s", "raring", "-r", "region-2",
			"-u", "some-auth-url", "-d", s.metadataDir, "-m", "1.12"},
	)
	c.Assert(code, gc.Equals, 0)
	errOut := ctx.Stdout.(*bytes.Buffer).String()
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, gc.Matches, `Matching Tools Versions:.*Resolve Metadata.*`)
}

func (s *ValidateToolsMetadataSuite) TestJustDirectory(c *gc.C) {
	s.makeLocalMetadata(c, "released", version.Current.Number.String(), "region-2", "raring", "some-auth-url")
	ctx := coretesting.Context(c)
	code := cmd.Main(
		envcmd.Wrap(&ValidateToolsMetadataCommand{}), ctx, []string{"-s", "raring", "-d", s.metadataDir},
	)
	c.Assert(code, gc.Equals, 0)
	errOut := ctx.Stdout.(*bytes.Buffer).String()
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, gc.Matches, `Matching Tools Versions:.*Resolve Metadata.*`)
}
