// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnet_test

import (
	"net"
	"regexp"
	stdtesting "testing"

	"github.com/juju/cmd"
	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/featureflag"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/subnet"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/network"
	coretesting "github.com/juju/juju/testing"
)

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

// BaseSubnetSuite is used for embedding in other suites.
type BaseSubnetSuite struct {
	coretesting.FakeJujuHomeSuite
	coretesting.BaseSuite

	superCmd cmd.Command
	command  cmd.Command
	api      *StubAPI
}

var _ = gc.Suite(&BaseSubnetSuite{})

func (s *BaseSubnetSuite) SetUpTest(c *gc.C) {
	// If any post-MVP command suite enabled the flag, keep it.
	hasFeatureFlag := featureflag.Enabled(feature.PostNetCLIMVP)

	s.BaseSuite.SetUpTest(c)
	s.FakeJujuHomeSuite.SetUpTest(c)

	if hasFeatureFlag {
		s.BaseSuite.SetFeatureFlags(feature.PostNetCLIMVP)
	}

	s.superCmd = subnet.NewSuperCommand()
	c.Assert(s.superCmd, gc.NotNil)

	s.api = NewStubAPI()
	c.Assert(s.api, gc.NotNil)

	// All subcommand suites embedding this one should initialize
	// s.command immediately after calling this method!
}

// RunSuperCommand executes the super command passing any args and
// returning the stdout and stderr output as strings, as well as any
// error. If s.command is set, the subcommand's name will be passed as
// first argument.
func (s *BaseSubnetSuite) RunSuperCommand(c *gc.C, args ...string) (string, string, error) {
	if s.command != nil {
		args = append([]string{s.command.Info().Name}, args...)
	}
	ctx, err := coretesting.RunCommand(c, s.superCmd, args...)
	if ctx != nil {
		return coretesting.Stdout(ctx), coretesting.Stderr(ctx), err
	}
	return "", "", err
}

// RunSubCommand executes the s.command subcommand passing any args
// and returning the stdout and stderr output as strings, as well as
// any error.
func (s *BaseSubnetSuite) RunSubCommand(c *gc.C, args ...string) (string, string, error) {
	if s.command == nil {
		panic("subcommand is nil")
	}
	ctx, err := coretesting.RunCommand(c, s.command, args...)
	if ctx != nil {
		return coretesting.Stdout(ctx), coretesting.Stderr(ctx), err
	}
	return "", "", err
}

// AssertRunFails is a shortcut for calling RunSubCommand with the
// passed args then asserting the output is empty and the error is as
// expected, finally returning the error.
func (s *BaseSubnetSuite) AssertRunFails(c *gc.C, expectErr string, args ...string) error {
	stdout, stderr, err := s.RunSubCommand(c, args...)
	c.Assert(err, gc.ErrorMatches, expectErr)
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")
	return err
}

// AssertRunSucceeds is a shortcut for calling RunSuperCommand with
// the passed args then asserting the stderr output matches
// expectStderr, stdout is equal to expectStdout, and the error is
// nil.
func (s *BaseSubnetSuite) AssertRunSucceeds(c *gc.C, expectStderr, expectStdout string, args ...string) {
	stdout, stderr, err := s.RunSubCommand(c, args...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stdout, gc.Equals, expectStdout)
	c.Assert(stderr, gc.Matches, expectStderr)
}

// TestHelp runs the command with --help as argument and verifies the
// output.
func (s *BaseSubnetSuite) TestHelp(c *gc.C) {
	stderr, stdout, err := s.RunSuperCommand(c, "--help")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(stdout, gc.Equals, "")
	c.Check(stderr, gc.Not(gc.Equals), "")

	// If s.command is set, use it instead of s.superCmd.
	cmdInfo := s.superCmd.Info()
	var expected string
	if s.command != nil {
		// Subcommands embed EnvCommandBase and have an extra
		// "[options]" prepended before the args.
		cmdInfo = s.command.Info()
		expected = "(?sm).*^usage: juju subnet " +
			regexp.QuoteMeta(cmdInfo.Name) +
			`( \[options\])? ` + regexp.QuoteMeta(cmdInfo.Args) + ".+"
	} else {
		expected = "(?sm).*^usage: juju subnet" +
			`( \[options\])? ` + regexp.QuoteMeta(cmdInfo.Args) + ".+"
	}
	c.Check(cmdInfo, gc.NotNil)
	c.Check(stderr, gc.Matches, expected)

	expected = "(?sm).*^purpose: " + regexp.QuoteMeta(cmdInfo.Purpose) + "$.*"
	c.Check(stderr, gc.Matches, expected)

	expected = "(?sm).*^" + regexp.QuoteMeta(cmdInfo.Doc) + "$.*"
	c.Check(stderr, gc.Matches, expected)
}

// Strings makes tests taking a slice of strings slightly easier to
// write: e.g. s.Strings("foo", "bar") vs. []string{"foo", "bar"}.
func (s *BaseSubnetSuite) Strings(values ...string) []string {
	return values
}

// StubAPI defines a testing stub for the SubnetAPI interface.
type StubAPI struct {
	*testing.Stub

	Subnets []params.Subnet
	Spaces  []names.Tag
	Zones   []string
}

var _ subnet.SubnetAPI = (*StubAPI)(nil)

// NewStubAPI creates a StubAPI suitable for passing to
// subnet.New*Command().
func NewStubAPI() *StubAPI {
	subnets := []params.Subnet{{
		// IPv4 subnet.
		CIDR:              "10.20.0.0/24",
		ProviderId:        "subnet-foo",
		Life:              params.Alive,
		SpaceTag:          "space-public",
		Zones:             []string{"zone1", "zone2"},
		StaticRangeLowIP:  net.ParseIP("10.20.0.10"),
		StaticRangeHighIP: net.ParseIP("10.20.0.100"),
	}, {
		// IPv6 subnet.
		CIDR:       "2001:db8::/32",
		ProviderId: "subnet-bar",
		Life:       params.Dying,
		SpaceTag:   "space-dmz",
		Zones:      []string{"zone2"},
	}, {
		// IPv4 VLAN subnet.
		CIDR:     "10.10.0.0/16",
		Life:     params.Dead,
		SpaceTag: "space-vlan-42",
		Zones:    []string{"zone1"},
		VLANTag:  42,
	}}
	return &StubAPI{
		Stub:    &testing.Stub{},
		Zones:   []string{"zone1", "zone2"},
		Subnets: subnets,
		Spaces: []names.Tag{
			names.NewSpaceTag("default"),
			names.NewSpaceTag("public"),
			names.NewSpaceTag("dmz"),
			names.NewSpaceTag("vlan-42"),
		},
	}
}

func (sa *StubAPI) Close() error {
	sa.MethodCall(sa, "Close")
	return sa.NextErr()
}

func (sa *StubAPI) AllZones() ([]string, error) {
	sa.MethodCall(sa, "AllZones")
	if err := sa.NextErr(); err != nil {
		return nil, err
	}
	return sa.Zones, nil
}

func (sa *StubAPI) AllSpaces() ([]names.Tag, error) {
	sa.MethodCall(sa, "AllSpaces")
	if err := sa.NextErr(); err != nil {
		return nil, err
	}
	return sa.Spaces, nil
}

func (sa *StubAPI) CreateSubnet(subnetCIDR names.SubnetTag, spaceTag names.SpaceTag, zones []string, isPublic bool) error {
	sa.MethodCall(sa, "CreateSubnet", subnetCIDR, spaceTag, zones, isPublic)
	return sa.NextErr()
}

func (sa *StubAPI) AddSubnet(cidr names.SubnetTag, id network.Id, spaceTag names.SpaceTag, zones []string) error {
	sa.MethodCall(sa, "AddSubnet", cidr, id, spaceTag, zones)
	return sa.NextErr()
}

func (sa *StubAPI) RemoveSubnet(subnetCIDR names.SubnetTag) error {
	sa.MethodCall(sa, "RemoveSubnet", subnetCIDR)
	return sa.NextErr()
}

func (sa *StubAPI) ListSubnets(withSpace *names.SpaceTag, withZone string) ([]params.Subnet, error) {
	if withSpace == nil {
		// Due to the way CheckCall works (using jc.DeepEquals
		// internally), we need to pass an explicit nil here, rather
		// than a pointer to a names.SpaceTag pointing to nil.
		sa.MethodCall(sa, "ListSubnets", nil, withZone)
	} else {
		sa.MethodCall(sa, "ListSubnets", withSpace, withZone)
	}
	if err := sa.NextErr(); err != nil {
		return nil, err
	}
	return sa.Subnets, nil
}
