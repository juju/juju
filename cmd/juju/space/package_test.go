// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space_test

import (
	"context"
	"strings"
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/cmd/juju/space"
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

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/spacesapi_mock.go github.com/juju/juju/cmd/juju/space SpaceAPI,SubnetAPI,API

// BaseSpaceSuite is used for embedding in other suites.
type BaseSpaceSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	coretesting.BaseSuite

	newCommand func() modelcmd.ModelCommand
	api        *StubAPI
}

func TestBaseSpaceSuite(t *stdtesting.T) {
	tc.Run(t, &BaseSpaceSuite{})
}

func (s *BaseSpaceSuite) SetUpSuite(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpSuite(c)
	s.BaseSuite.SetUpSuite(c)
}

func (s *BaseSpaceSuite) TearDownSuite(c *tc.C) {
	s.BaseSuite.TearDownSuite(c)
	s.FakeJujuXDGDataHomeSuite.TearDownSuite(c)
}

func (s *BaseSpaceSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)

	s.api = NewStubAPI()
	c.Assert(s.api, tc.NotNil)

	// All subcommand suites embedding this one should initialize
	// s.newCommand immediately after calling this method!
}

func (s *BaseSpaceSuite) TearDownTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

// InitCommand creates a command with s.newCommand and runs its
// Init method only. It returns the inner command and any error.
func (s *BaseSpaceSuite) InitCommand(c *tc.C, args ...string) (cmd.Command, error) {
	cmd := s.newCommandForTest()
	err := cmdtesting.InitCommand(cmd, args)
	return modelcmd.InnerCommand(cmd), err
}

// RunCommand creates a command with s.newCommand and executes it,
// passing any args and returning the stdout and stderr output as
// strings, as well as any error.
func (s *BaseSpaceSuite) RunCommand(c *tc.C, args ...string) (string, string, error) {
	cmd := s.newCommandForTest()
	ctx, err := cmdtesting.RunCommand(c, cmd, args...)
	return cmdtesting.Stdout(ctx), cmdtesting.Stderr(ctx), err
}

func (s *BaseSpaceSuite) newCommandForTest() modelcmd.ModelCommand {
	cmd := s.newCommand()
	// The client store shouldn't be used, but mock it
	// out to make sure.
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	cmd1 := modelcmd.InnerCommand(cmd).(interface {
		SetAPI(space.API)
	})
	cmd1.SetAPI(s.api)
	return cmd
}

// AssertRunSpacesNotSupported is a shortcut for calling RunCommand with the
// passed args then asserting the output is empty and the error is the
// spaces not supported, finally returning the error.
func (s *BaseSpaceSuite) AssertRunSpacesNotSupported(c *tc.C, expectErr string, args ...string) error {
	stdout, stderr, err := s.RunCommand(c, args...)
	c.Assert(err, tc.ErrorMatches, expectErr)
	c.Assert(stdout, tc.Equals, "")
	c.Assert(stderr, tc.Equals, expectErr+"\n")
	return err
}

// AssertRunFailsUnauthoirzed is a shortcut for calling RunCommand with the
// passed args then asserting the error is as expected, finally returning the
// error.
func (s *BaseSpaceSuite) AssertRunFailsUnauthorized(c *tc.C, expectErr string, args ...string) error {
	_, stderr, err := s.RunCommand(c, args...)
	c.Assert(strings.Replace(stderr, "\n", " ", -1), tc.Matches, `.*juju grant.*`)
	return err
}

// AssertRunFails is a shortcut for calling RunCommand with the
// passed args then asserting the output is empty and the error is as
// expected, finally returning the error.
func (s *BaseSpaceSuite) AssertRunFails(c *tc.C, expectErr string, args ...string) error {
	stdout, stderr, err := s.RunCommand(c, args...)
	c.Assert(err, tc.ErrorMatches, expectErr)
	c.Assert(stdout, tc.Equals, "")
	c.Assert(stderr, tc.Equals, "")
	return err
}

// AssertRunSucceeds is a shortcut for calling RunSuperCommand with
// the passed args then asserting the stderr output matches
// expectStderr, stdout is equal to expectStdout, and the error is
// nil.
func (s *BaseSpaceSuite) AssertRunSucceeds(c *tc.C, expectStderr, expectStdout string, args ...string) {
	stdout, stderr, err := s.RunCommand(c, args...)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(stdout, tc.Equals, expectStdout)
	c.Assert(stderr, tc.Matches, expectStderr)
}

// Strings is makes tests taking a slice of strings slightly easier to
// write: e.g. s.Strings("foo", "bar") vs. []string{"foo", "bar"}.
func (s *BaseSpaceSuite) Strings(values ...string) []string {
	return values
}

// StubAPI defines a testing stub for the SpaceAPI interface.
type StubAPI struct {
	*testhelpers.Stub

	Spaces  []params.Space
	Subnets []params.Subnet

	ShowSpaceResp     params.ShowSpaceResult
	MoveSubnetsResp   params.MoveSubnetsResult
	SubnetsByCIDRResp []params.SubnetsResult
}

var _ space.API = (*StubAPI)(nil)

// NewStubAPI creates a StubAPI suitable for passing to
// space.New*Command().
func NewStubAPI() *StubAPI {
	subnets := []params.Subnet{{
		// IPv6 subnet.
		CIDR:       "2001:db8::/32",
		ProviderId: "subnet-public",
		Life:       life.Dying,
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
		CIDR:       "10.1.2.0/24",
		ProviderId: "subnet-private",
		Life:       life.Alive,
		SpaceTag:   "space-space2",
		Zones:      []string{"zone1", "zone2"},
	}, {
		// IPv4 VLAN subnet.
		CIDR:       "4.3.2.0/28",
		Life:       life.Dead,
		ProviderId: "vlan-42",
		SpaceTag:   "space-space2",
		Zones:      []string{"zone1"},
		VLANTag:    42,
	}}
	spaces := []params.Space{{
		Id:   network.AlphaSpaceId,
		Name: network.AlphaSpaceName,
	}, {
		Id:      "deadbeef1",
		Name:    "space1",
		Subnets: append([]params.Subnet{}, subnets[:2]...),
	}, {
		Id:      "deadbeef2",
		Name:    "space2",
		Subnets: append([]params.Subnet{}, subnets[2:]...),
	}}
	showSpace := params.ShowSpaceResult{
		Space: params.Space{
			Id:   spaces[1].Id,
			Name: spaces[1].Name,
			Subnets: []params.Subnet{{
				CIDR: subnets[0].CIDR,
			}, {
				CIDR: subnets[2].CIDR,
			}},
		},
	}
	moveSubnets := params.MoveSubnetsResult{
		MovedSubnets: []params.MovedSubnet{{
			SubnetTag:   "0195847b-95bb-7ca1-a7ee-2211d802d5b3",
			OldSpaceTag: "space-internal",
			CIDR:        subnets[0].CIDR,
		}},
		NewSpaceTag: "space-public",
	}
	subnetsByCIDR := []params.SubnetsResult{{
		Subnets: []params.SubnetV2{{
			ID:     "0195847b-95bb-7ca1-a7ee-2211d802d5b3",
			Subnet: subnets[0],
		}, {
			ID:     "0195847b-95bb-7ca1-a7ee-2211d802d5b4",
			Subnet: subnets[2],
		}},
	}}
	return &StubAPI{
		Stub:              &testhelpers.Stub{},
		Spaces:            spaces,
		Subnets:           subnets,
		ShowSpaceResp:     showSpace,
		MoveSubnetsResp:   moveSubnets,
		SubnetsByCIDRResp: subnetsByCIDR,
	}
}

func (sa *StubAPI) Close() error {
	sa.MethodCall(sa, "Close")
	return sa.NextErr()
}

func (sa *StubAPI) ListSpaces(ctx context.Context) ([]params.Space, error) {
	sa.MethodCall(sa, "ListSpaces")
	if err := sa.NextErr(); err != nil {
		return nil, err
	}
	return sa.Spaces, nil
}

func (sa *StubAPI) AddSpace(ctx context.Context, name string, subnetIds []string, public bool) error {
	sa.MethodCall(sa, "AddSpace", name, subnetIds, public)
	return sa.NextErr()
}

func (sa *StubAPI) RemoveSpace(ctx context.Context, name string, force bool, dryRun bool) (params.RemoveSpaceResult, error) {
	sa.MethodCall(sa, "RemoveSpace", name)
	return params.RemoveSpaceResult{}, sa.NextErr()
}

func (sa *StubAPI) RenameSpace(ctx context.Context, name, newName string) error {
	sa.MethodCall(sa, "RenameSpace", name, newName)
	return sa.NextErr()
}

func (sa *StubAPI) ReloadSpaces(ctx context.Context) error {
	sa.MethodCall(sa, "ReloadSpaces")
	return sa.NextErr()
}

func (sa *StubAPI) ShowSpace(ctx context.Context, name string) (params.ShowSpaceResult, error) {
	sa.MethodCall(sa, "ShowSpace", name)
	if err := sa.NextErr(); err != nil {
		return params.ShowSpaceResult{}, err
	}
	return sa.ShowSpaceResp, nil
}

func (sa *StubAPI) MoveSubnets(ctx context.Context, name names.SpaceTag, tags []names.SubnetTag, force bool) (params.MoveSubnetsResult, error) {
	sa.MethodCall(sa, "MoveSubnets", name, tags, force)
	return sa.MoveSubnetsResp, sa.NextErr()
}

func (sa *StubAPI) SubnetsByCIDR(ctx context.Context, cidrs []string) ([]params.SubnetsResult, error) {
	sa.MethodCall(sa, "SubnetsByCIDR", cidrs)
	return sa.SubnetsByCIDRResp, sa.NextErr()
}
