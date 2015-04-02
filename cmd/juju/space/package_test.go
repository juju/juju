// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space_test

import (
	"net"
	"regexp"
	stdtesting "testing"

	"github.com/juju/cmd"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

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

	Subnets []network.SubnetInfo
}

var _ space.SpaceAPI = (*StubAPI)(nil)

// NewStubAPI creates a StubAPI suitable for passing to
// space.New*Command().
func NewStubAPI() *StubAPI {
	return &StubAPI{
		Stub: &testing.Stub{},
		Subnets: []network.SubnetInfo{{
			CIDR:              "10.1.2.0/24",
			ProviderId:        "subnet-private",
			AllocatableIPLow:  net.ParseIP("10.1.2.10"),
			AllocatableIPHigh: net.ParseIP("10.1.2.200"),
		}, {
			CIDR:       "0.1.0.0/16",
			ProviderId: "subnet-public",
		}, {
			CIDR:              "4.3.2.0/28",
			ProviderId:        "vlan-42",
			VLANTag:           42,
			AllocatableIPLow:  net.ParseIP("4.3.2.2"),
			AllocatableIPHigh: net.ParseIP("4.3.2.4"),
		}},
	}
}

func (sa *StubAPI) Close() error {
	sa.MethodCall(sa, "Close")
	return sa.NextErr()
}

func (sa *StubAPI) AllSubnets() ([]network.SubnetInfo, error) {
	sa.MethodCall(sa, "AllSubnets")
	if err := sa.NextErr(); err != nil {
		return nil, err
	}
	return sa.Subnets, nil
}

func (sa *StubAPI) CreateSpace(name string, subnetIds []string) error {
	sa.MethodCall(sa, "CreateSpace", name, subnetIds)
	return sa.NextErr()
}

func (sa *StubAPI) RemoveSpace(name string) error {
	sa.MethodCall(sa, "RemoveSpace", name)
	return sa.NextErr()
}

func (sa *StubAPI) UpdateSpace(name string, subnetIds []string) error {
	sa.MethodCall(sa, "UpdateSpace", name, subnetIds)
	return sa.NextErr()
}

func (sa *StubAPI) RenameSpace(name, newName string) error {
	sa.MethodCall(sa, "RenameSpace", name, newName)
	return sa.NextErr()
}
