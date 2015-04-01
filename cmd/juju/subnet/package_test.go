// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnet_test

import (
	"regexp"
	stdtesting "testing"

	"github.com/juju/cmd"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/subnet"
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
	s.BaseSuite.SetUpTest(c)
	s.FakeJujuHomeSuite.SetUpTest(c)

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
// expectStderr, stdout is empty and the error is nil.
func (s *BaseSubnetSuite) AssertRunSucceeds(c *gc.C, expectStderr string, args ...string) {
	stdout, stderr, err := s.RunSubCommand(c, args...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stdout, gc.Equals, "")
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
			` \[options\] ` + regexp.QuoteMeta(cmdInfo.Args) + ".+"
	} else {
		expected = "(?sm).*^usage: juju subnet " +
			regexp.QuoteMeta(cmdInfo.Args) + ".+"
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

	Zones []string
}

var _ subnet.SubnetAPI = (*StubAPI)(nil)

// NewStubAPI creates a StubAPI suitable for passing to
// subnet.New*Command().
func NewStubAPI() *StubAPI {
	return &StubAPI{
		Stub:  &testing.Stub{},
		Zones: []string{"zone1", "zone2"},
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

func (sa *StubAPI) CreateSubnet(subnetCIDR, spaceName string, zones []string, isPublic bool) error {
	sa.MethodCall(sa, "CreateSubnet", subnetCIDR, spaceName, zones, isPublic)
	return sa.NextErr()
}

func (sa *StubAPI) AddSubnet(subnetCIDR, spaceName string) error {
	sa.MethodCall(sa, "AddSubnet", subnetCIDR, spaceName)
	return sa.NextErr()
}

func (sa *StubAPI) RemoveSubnet(subnetCIDR string) error {
	sa.MethodCall(sa, "RemoveSubnet", subnetCIDR)
	return sa.NextErr()
}
