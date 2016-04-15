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
	"github.com/juju/utils/featureflag"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/space"
	"github.com/juju/juju/feature"
	coretesting "github.com/juju/juju/testing"
)

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

// BaseSpaceSuite is used for embedding in other suites.
type BaseSpaceSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	coretesting.BaseSuite

	command  cmd.Command
	api      *StubAPI
}

var _ = gc.Suite(&BaseSpaceSuite{})

func (s *BaseSpaceSuite) SetUpSuite(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpSuite(c)
	s.BaseSuite.SetUpSuite(c)
}

func (s *BaseSpaceSuite) TearDownSuite(c *gc.C) {
	s.BaseSuite.TearDownSuite(c)
	s.FakeJujuXDGDataHomeSuite.TearDownSuite(c)
}

func (s *BaseSpaceSuite) SetUpTest(c *gc.C) {
	// If any post-MVP command suite enabled the flag, keep it.
	hasFeatureFlag := featureflag.Enabled(feature.PostNetCLIMVP)

	s.BaseSuite.SetUpTest(c)
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)

	if hasFeatureFlag {
		s.BaseSuite.SetFeatureFlags(feature.PostNetCLIMVP)
	}

	s.api = NewStubAPI()
	c.Assert(s.api, gc.NotNil)

	// All subcommand suites embedding this one should initialize
	// s.command immediately after calling this method!
}

func (s *BaseSpaceSuite) TearDownTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

// RunCommand executes the s.command subcommand passing any args
// and returning the stdout and stderr output as strings, as well as
// any error.
func (s *BaseSpaceSuite) RunCommand(c *gc.C, args ...string) (string, string, error) {
	if s.command == nil {
		panic("subcommand is nil")
	}
	ctx, err := coretesting.RunCommand(c, s.command, args...)
	if ctx != nil {
		return coretesting.Stdout(ctx), coretesting.Stderr(ctx), err
	}
	return "", "", err
}

// AssertRunSpacesNotSupported is a shortcut for calling RunSubCommand with the
// passed args then asserting the output is empty and the error is the
// spaces not supported, finally returning the error.
func (s *BaseSpaceSuite) AssertRunSpacesNotSupported(c *gc.C, expectErr string, args ...string) error {
	stdout, stderr, err := s.RunSubCommand(c, args...)
	c.Assert(err, gc.ErrorMatches, expectErr)
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, expectErr+"\n")
	return err
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

// Strings is makes tests taking a slice of strings slightly easier to
// write: e.g. s.Strings("foo", "bar") vs. []string{"foo", "bar"}.
func (s *BaseSpaceSuite) Strings(values ...string) []string {
	return values
}

// StubAPI defines a testing stub for the SpaceAPI interface.
type StubAPI struct {
	*testing.Stub

	Spaces  []params.Space
	Subnets []params.Subnet
}

var _ space.SpaceAPI = (*StubAPI)(nil)

// NewStubAPI creates a StubAPI suitable for passing to
// space.New*Command().
func NewStubAPI() *StubAPI {
	subnets := []params.Subnet{{
		// IPv6 subnet.
		CIDR:       "2001:db8::/32",
		ProviderId: "subnet-public",
		Life:       params.Dying,
		SpaceTag:   "space-space1",
		Zones:      []string{"zone2"},
	}, {
		// Invalid subnet (just for 100% coverage, otherwise it can't happen).
		CIDR:       "invalid",
		ProviderId: "no-such",
		SpaceTag:   "space-space1",
		Zones:      []string{"zone1"},
	}, {
		// IPv4 subnet.
		CIDR:              "10.1.2.0/24",
		ProviderId:        "subnet-private",
		Life:              params.Alive,
		SpaceTag:          "space-space2",
		Zones:             []string{"zone1", "zone2"},
		StaticRangeLowIP:  net.ParseIP("10.1.2.10"),
		StaticRangeHighIP: net.ParseIP("10.1.2.200"),
	}, {
		// IPv4 VLAN subnet.
		CIDR:              "4.3.2.0/28",
		Life:              params.Dead,
		ProviderId:        "vlan-42",
		SpaceTag:          "space-space2",
		Zones:             []string{"zone1"},
		VLANTag:           42,
		StaticRangeLowIP:  net.ParseIP("4.3.2.2"),
		StaticRangeHighIP: net.ParseIP("4.3.2.4"),
	}}
	spaces := []params.Space{{
		Name:    "space1",
		Subnets: append([]params.Subnet{}, subnets[:2]...),
	}, {
		Name:    "space2",
		Subnets: append([]params.Subnet{}, subnets[2:]...),
	}}
	return &StubAPI{
		Stub:    &testing.Stub{},
		Spaces:  spaces,
		Subnets: subnets,
	}
}

func (sa *StubAPI) Close() error {
	sa.MethodCall(sa, "Close")
	return sa.NextErr()
}

func (sa *StubAPI) ListSpaces() ([]params.Space, error) {
	sa.MethodCall(sa, "ListSpaces")
	if err := sa.NextErr(); err != nil {
		return nil, err
	}
	return sa.Spaces, nil
}

func (sa *StubAPI) CreateSpace(name string, subnetIds []string, public bool) error {
	sa.MethodCall(sa, "CreateSpace", name, subnetIds, public)
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
