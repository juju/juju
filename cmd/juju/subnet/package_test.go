// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnet_test

import (
	"context"
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/cmd/juju/subnet"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/rpc/params"
)

// BaseSubnetSuite is used for embedding in other suites.
type BaseSubnetSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite

	newCommand func() modelcmd.ModelCommand
	api        *StubAPI
}

func TestBaseSubnetSuite(t *stdtesting.T) {
	tc.Run(t, &BaseSubnetSuite{})
}

func (s *BaseSubnetSuite) SetUpSuite(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpSuite(c)
}

func (s *BaseSubnetSuite) TearDownSuite(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.TearDownSuite(c)
}

func (s *BaseSubnetSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)

	s.api = NewStubAPI()
	c.Assert(s.api, tc.NotNil)

	// All subcommand suites embedding this one should initialize
	// s.newCommand immediately after calling this method!
}

func (s *BaseSubnetSuite) TearDownTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
}

// InitCommand creates a command with s.newCommand and runs its
// Init method only. It returns the inner command and any error.
func (s *BaseSubnetSuite) InitCommand(c *tc.C, args ...string) (cmd.Command, error) {
	cmd := s.newCommandForTest()
	err := cmdtesting.InitCommand(cmd, args)
	return modelcmd.InnerCommand(cmd), err
}

// RunCommand creates a command with s.newCommand and executes it,
// passing any args and returning the stdout and stderr output as
// strings, as well as any error.
func (s *BaseSubnetSuite) RunCommand(c *tc.C, args ...string) (string, string, error) {
	cmd := s.newCommandForTest()
	ctx, err := cmdtesting.RunCommand(c, cmd, args...)
	return cmdtesting.Stdout(ctx), cmdtesting.Stderr(ctx), err
}

func (s *BaseSubnetSuite) newCommandForTest() modelcmd.ModelCommand {
	cmd := s.newCommand()
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	cmd1 := modelcmd.InnerCommand(cmd).(interface {
		SetAPI(subnet.SubnetAPI)
	})
	cmd1.SetAPI(s.api)
	return cmd
}

// AssertRunFails is a shortcut for calling RunCommand with the
// passed args then asserting the output is empty and the error is as
// expected, finally returning the error.
func (s *BaseSubnetSuite) AssertRunFails(c *tc.C, expectErr string, args ...string) error {
	stdout, stderr, err := s.RunCommand(c, args...)
	c.Assert(err, tc.ErrorMatches, expectErr)
	c.Assert(stdout, tc.Equals, "")
	c.Assert(stderr, tc.Equals, "")
	return err
}

// AssertRunSucceeds is a shortcut for calling RunCommand with
// the passed args then asserting the stderr output matches
// expectStderr, stdout is equal to expectStdout, and the error is
// nil.
func (s *BaseSubnetSuite) AssertRunSucceeds(c *tc.C, expectStderr, expectStdout string, args ...string) {
	stdout, stderr, err := s.RunCommand(c, args...)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(stdout, tc.Equals, expectStdout)
	c.Assert(stderr, tc.Matches, expectStderr)
}

// Strings makes tests taking a slice of strings slightly easier to
// write: e.g. s.Strings("foo", "bar") vs. []string{"foo", "bar"}.
func (s *BaseSubnetSuite) Strings(values ...string) []string {
	return values
}

// StubAPI defines a testing stub for the SubnetAPI interface.
type StubAPI struct {
	*testhelpers.Stub

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
		CIDR:       "10.20.0.0/24",
		ProviderId: "subnet-foo",
		Life:       life.Alive,
		SpaceTag:   "space-public",
		Zones:      []string{"zone1", "zone2"},
	}, {
		// IPv6 subnet.
		CIDR:              "2001:db8::/32",
		ProviderId:        "subnet-bar",
		ProviderNetworkId: "network-yay",
		Life:              life.Dying,
		SpaceTag:          "space-dmz",
		Zones:             []string{"zone2"},
	}, {
		// IPv4 VLAN subnet.
		CIDR:     "10.10.0.0/16",
		Life:     life.Dead,
		SpaceTag: "space-vlan-42",
		Zones:    []string{"zone1"},
		VLANTag:  42,
	}}
	return &StubAPI{
		Stub:    &testhelpers.Stub{},
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

func (sa *StubAPI) AddSubnet(ctx context.Context, cidr string, id network.Id, spaceTag names.SpaceTag, zones []string) error {
	sa.MethodCall(sa, "AddSubnet", cidr, id, spaceTag, zones)
	return sa.NextErr()
}

func (sa *StubAPI) ListSubnets(ctx context.Context, withSpace *names.SpaceTag, withZone string) ([]params.Subnet, error) {
	if withSpace == nil {
		// Due to the way CheckCall works (using tc.DeepEquals
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
