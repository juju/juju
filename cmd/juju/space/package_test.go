// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space_test

import (
	"net"
	"regexp"
	"strings"
	stdtesting "testing"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/space"
	"github.com/juju/juju/network"
	coretesting "github.com/juju/juju/testing"
)

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

// BaseSpaceSuite is used for embedding in other suites.
type BaseSpaceSuite struct {
	coretesting.FakeJujuHomeSuite
	coretesting.BaseSuite

	superCmd cmd.Command
	command  cmd.Command
	api      *StubAPI
}

var _ = gc.Suite(&BaseSpaceSuite{})

func (s *BaseSpaceSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.FakeJujuHomeSuite.SetUpTest(c)

	s.superCmd = space.NewSuperCommand()
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
func (s *BaseSpaceSuite) RunSuperCommand(c *gc.C, args ...string) (string, string, error) {
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
func (s *BaseSpaceSuite) RunSubCommand(c *gc.C, args ...string) (string, string, error) {
	if s.command == nil {
		panic("subcommand is nil")
	}
	ctx, err := coretesting.RunCommand(c, s.command, args...)
	if ctx != nil {
		return coretesting.Stdout(ctx), coretesting.Stderr(ctx), err
	}
	return "", "", err
}

func (s *BaseSpaceSuite) CheckOutputs(
	c *gc.C, stdout, stderr string, err error,
	expectedStdout, expectedStderr, expectedErr string) {
	if expectedErr == "" {
		c.Assert(err, jc.ErrorIsNil)
	} else {
		c.Assert(err, gc.ErrorMatches, expectedErr)
	}
	c.Assert(stdout, gc.Equals, expectedStdout)
	c.Assert(stderr, gc.Matches, expectedStderr)
}

func (s *BaseSpaceSuite) CheckOutputsStderr(c *gc.C, stdout, stderr string, err error, expectedStderr string) {
	s.CheckOutputs(c, stdout, stderr, err, "", expectedStderr, "")
}

func (s *BaseSpaceSuite) CheckOutputsErr(c *gc.C, stdout, stderr string, err error, expectedErr string) {
	s.CheckOutputs(c, stdout, stderr, err, "", "", expectedErr)
}

func (s *BaseSpaceSuite) CheckOutputsStdout(c *gc.C, stdout, stderr string, err error, expectedStdout string) {
	s.CheckOutputs(c, stdout, stderr, err, expectedStdout, "", "")
}

// AssertRunFails is a shortcut for calling RunSubCommand with the
// passed args then asserting the output is empty and the error is as
// expected, finally returning the error.
func (s *BaseSpaceSuite) AssertRunFails(c *gc.C, expectErr string, args ...string) error {
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
func (s *BaseSpaceSuite) AssertRunSucceeds(c *gc.C, expectStderr, expectStdout string, args ...string) {
	stdout, stderr, err := s.RunSubCommand(c, args...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stdout, gc.Equals, expectStdout)
	c.Assert(stderr, gc.Matches, expectStderr)
}

// TestHelp runs the command with --help as argument and verifies the
// output.
func (s *BaseSpaceSuite) TestHelp(c *gc.C) {
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
		expected = "(?sm).*^usage: juju space " +
			regexp.QuoteMeta(cmdInfo.Name) +
			` \[options\] ` + regexp.QuoteMeta(cmdInfo.Args) + ".+"
	} else {
		expected = "(?sm).*^usage: juju space " +
			regexp.QuoteMeta(cmdInfo.Args) + ".+"
	}
	c.Check(cmdInfo, gc.NotNil)
	c.Check(stderr, gc.Matches, expected)

	expected = "(?sm).*^purpose: " + regexp.QuoteMeta(cmdInfo.Purpose) + "$.*"
	c.Check(stderr, gc.Matches, expected)

	expected = "(?sm).*^" + regexp.QuoteMeta(cmdInfo.Doc) + "$.*"
	c.Check(stderr, gc.Matches, expected)
}

// Strings is makes tests taking a slice of strings slightly easier to
// write: e.g. s.Strings("foo", "bar") vs. []string{"foo", "bar"}.
func (s *BaseSpaceSuite) Strings(values ...string) []string {
	return values
}

// StubAPI defines a testing stub for the SpaceAPI interface.
type StubAPI struct {
	*testing.Stub

	//Subnets  []network.SubnetInfo
	Spaces  []network.SpaceInfo
	Subnets []params.Subnet
}

var _ space.SpaceAPI = (*StubAPI)(nil)

// NewStubAPI creates a StubAPI suitable for passing to
// space.New*Command().
func NewStubAPI() *StubAPI {
	return &StubAPI{
		Stub: &testing.Stub{},
		/*Subnets: []network.SubnetInfo{{
			CIDR:              "10.1.2.0/24",
			ProviderId:        "subnet-private",
			AllocatableIPLow:  net.ParseIP("10.1.2.10"),
			AllocatableIPHigh: net.ParseIP("10.1.2.200"),
		}, {
			CIDR:       "2001:db8::/32",
			ProviderId: "subnet-public",
		}, {
			CIDR:              "4.3.2.0/28",
			ProviderId:        "vlan-42",
			VLANTag:           42,
			AllocatableIPLow:  net.ParseIP("4.3.2.2"),
			AllocatableIPHigh: net.ParseIP("4.3.2.4"),
		}},*/
		Spaces: []network.SpaceInfo{{
			Name:  "space1",
			CIDRs: []string{"10.1.2.0/24"},
		}, {
			Name:  "space2",
			CIDRs: []string{"10.1.2.0/24", "4.3.2.0/28"},
		}},
		Subnets: []params.Subnet{{
			// IPv4 subnet.
			CIDR:              "10.1.2.0/24",
			ProviderId:        "subnet-private",
			Life:              params.Alive,
			SpaceTag:          "space-public",
			Zones:             []string{"zone1", "zone2"},
			StaticRangeLowIP:  net.ParseIP("10.1.2.10"),
			StaticRangeHighIP: net.ParseIP("10.1.2.200"),
		}, {
			// IPv6 subnet.
			CIDR:       "10.1.2.0/24",
			ProviderId: "subnet-public",
			Life:       params.Dying,
			SpaceTag:   "space-dmz",
			Zones:      []string{"zone2"},
		}, {
			// IPv4 VLAN subnet.
			CIDR:              "4.3.2.0/28",
			Life:              params.Dead,
			ProviderId:        "vlan-42",
			SpaceTag:          "space-vlan-42",
			Zones:             []string{"zone1"},
			VLANTag:           42,
			StaticRangeLowIP:  net.ParseIP("4.3.2.2"),
			StaticRangeHighIP: net.ParseIP("4.3.2.4"),
		}},
	}
}

func (sa *StubAPI) Close() error {
	sa.MethodCall(sa, "Close")
	return sa.NextErr()
}

func (sa *StubAPI) AllSpaces() ([]network.SpaceInfo, error) {
	sa.MethodCall(sa, "AllSpaces")
	if err := sa.NextErr(); err != nil {
		return nil, err
	}
	return sa.Spaces, nil
}

func (sa *StubAPI) AllSubnets() ([]params.Subnet, error) {
	sa.MethodCall(sa, "AllSubnets")
	if err := sa.NextErr(); err != nil {
		return nil, err
	}
	return sa.Subnets, nil
}

func (sa *StubAPI) CreateSpace(name string, subnetIds []string) error {
	if err := sa.SubnetsExist(subnetIds); err != nil {
		return err
	}

	sa.MethodCall(sa, "CreateSpace", name, subnetIds)
	return sa.NextErr()
}

func (sa *StubAPI) RemoveSpace(name string) error {
	sa.MethodCall(sa, "RemoveSpace", name)
	return sa.NextErr()
}

func (sa *StubAPI) UpdateSpace(name string, subnetIds []string) error {
	if err := sa.SubnetsExist(subnetIds); err != nil {
		return err
	}

	sa.MethodCall(sa, "UpdateSpace", name, subnetIds)
	return sa.NextErr()
}

func (sa *StubAPI) RenameSpace(name, newName string) error {
	sa.MethodCall(sa, "RenameSpace", name, newName)
	return sa.NextErr()
}

func (sa *StubAPI) SubnetsExist(subnetIds []string) error {
	// Fetch all subnets to validate the given CIDRs.
	CIDRs := set.NewStrings(subnetIds...)
	subnets, err := sa.AllSubnets()
	if err != nil {
		return errors.Annotate(err, "cannot fetch available subnets")
	}

	// Find which of the given CIDRs match existing ones.
	validCIDRs := set.NewStrings()
	for _, subnet := range subnets {
		validCIDRs.Add(subnet.CIDR)
	}
	diff := CIDRs.Difference(validCIDRs)

	if !diff.IsEmpty() {
		// Some given CIDRs are missing.
		subnets := strings.Join(diff.SortedValues(), ", ")
		return errors.Errorf("unknown subnets specified: %s", subnets)
	}

	return nil
}
