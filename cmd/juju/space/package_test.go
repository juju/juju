// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space_test

import (
	"strings"
	stdtesting "testing"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/space"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/spacesapi_mock.go github.com/juju/juju/cmd/juju/space SpaceAPI,SubnetAPI,API

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

// BaseSpaceSuite is used for embedding in other suites.
type BaseSpaceSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	coretesting.BaseSuite

	newCommand func() modelcmd.ModelCommand
	api        *StubAPI
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
	s.BaseSuite.SetUpTest(c)
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)

	s.api = NewStubAPI()
	c.Assert(s.api, gc.NotNil)

	// All subcommand suites embedding this one should initialize
	// s.newCommand immediately after calling this method!
}

func (s *BaseSpaceSuite) TearDownTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

// InitCommand creates a command with s.newCommand and runs its
// Init method only. It returns the inner command and any error.
func (s *BaseSpaceSuite) InitCommand(c *gc.C, args ...string) (cmd.Command, error) {
	cmd := s.newCommandForTest()
	err := cmdtesting.InitCommand(cmd, args)
	return modelcmd.InnerCommand(cmd), err
}

// RunCommand creates a command with s.newCommand and executes it,
// passing any args and returning the stdout and stderr output as
// strings, as well as any error.
func (s *BaseSpaceSuite) RunCommand(c *gc.C, args ...string) (string, string, error) {
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
func (s *BaseSpaceSuite) AssertRunSpacesNotSupported(c *gc.C, expectErr string, args ...string) error {
	stdout, stderr, err := s.RunCommand(c, args...)
	c.Assert(err, gc.ErrorMatches, expectErr)
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, expectErr+"\n")
	return err
}

// AssertRunFailsUnauthoirzed is a shortcut for calling RunCommand with the
// passed args then asserting the error is as expected, finally returning the
// error.
func (s *BaseSpaceSuite) AssertRunFailsUnauthorized(c *gc.C, expectErr string, args ...string) error {
	_, stderr, err := s.RunCommand(c, args...)
	c.Assert(strings.Replace(stderr, "\n", " ", -1), gc.Matches, `.*juju grant.*`)
	return err
}

// AssertRunFails is a shortcut for calling RunCommand with the
// passed args then asserting the output is empty and the error is as
// expected, finally returning the error.
func (s *BaseSpaceSuite) AssertRunFails(c *gc.C, expectErr string, args ...string) error {
	stdout, stderr, err := s.RunCommand(c, args...)
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
	stdout, stderr, err := s.RunCommand(c, args...)
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
		Id:   "0",
		Name: network.AlphaSpaceName,
	}, {
		Id:      "1",
		Name:    "space1",
		Subnets: append([]params.Subnet{}, subnets[:2]...),
	}, {
		Id:      "2",
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
			SubnetTag:   "1",
			OldSpaceTag: "space-internal",
			CIDR:        subnets[0].CIDR,
		}},
		NewSpaceTag: "space-public",
	}
	subnetsByCIDR := []params.SubnetsResult{{
		Subnets: []params.SubnetV2{{
			ID:     "1",
			Subnet: subnets[0],
		}, {
			ID:     "2",
			Subnet: subnets[2],
		}},
	}}
	return &StubAPI{
		Stub:              &testing.Stub{},
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

func (sa *StubAPI) ListSpaces() ([]params.Space, error) {
	sa.MethodCall(sa, "ListSpaces")
	if err := sa.NextErr(); err != nil {
		return nil, err
	}
	return sa.Spaces, nil
}

func (sa *StubAPI) AddSpace(name string, subnetIds []string, public bool) error {
	sa.MethodCall(sa, "AddSpace", name, subnetIds, public)
	return sa.NextErr()
}

func (sa *StubAPI) RemoveSpace(name string, force bool, dryRun bool) (params.RemoveSpaceResult, error) {
	sa.MethodCall(sa, "RemoveSpace", name)
	return params.RemoveSpaceResult{}, sa.NextErr()
}

func (sa *StubAPI) RenameSpace(name, newName string) error {
	sa.MethodCall(sa, "RenameSpace", name, newName)
	return sa.NextErr()
}

func (sa *StubAPI) ReloadSpaces() error {
	sa.MethodCall(sa, "ReloadSpaces")
	return sa.NextErr()
}

func (sa *StubAPI) ShowSpace(name string) (params.ShowSpaceResult, error) {
	sa.MethodCall(sa, "ShowSpace", name)
	if err := sa.NextErr(); err != nil {
		return params.ShowSpaceResult{}, err
	}
	return sa.ShowSpaceResp, nil
}

func (sa *StubAPI) MoveSubnets(name names.SpaceTag, tags []names.SubnetTag, force bool) (params.MoveSubnetsResult, error) {
	sa.MethodCall(sa, "MoveSubnets", name, tags, force)
	return sa.MoveSubnetsResp, sa.NextErr()
}

func (sa *StubAPI) SubnetsByCIDR(cidrs []string) ([]params.SubnetsResult, error) {
	sa.MethodCall(sa, "SubnetsByCIDR", cidrs)
	return sa.SubnetsByCIDRResp, sa.NextErr()
}
