// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

type toolsSuite struct {
	testing.JujuConnSuite
	machine0 *state.Machine
}

var _ = gc.Suite(&toolsSuite{})

func (s *toolsSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	var err error
	s.machine0, err = s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
}

func (s *toolsSuite) TestTools(c *gc.C) {
	getCanRead := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return tag == names.NewMachineTag("0") || tag == names.NewMachineTag("42")
		}, nil
	}
	tg := common.NewToolsGetter(s.State, s.State, sprintfURLGetter("tools:%s"), getCanRead)
	c.Assert(tg, gc.NotNil)

	err := s.machine0.SetAgentVersion(version.Current)
	c.Assert(err, gc.IsNil)

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: "machine-0"},
			{Tag: "machine-1"},
			{Tag: "machine-42"},
		}}
	result, err := tg.Tools(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result.Results, gc.HasLen, 3)
	c.Assert(result.Results[0].Tools, gc.NotNil)
	c.Assert(result.Results[0].Tools.Version, gc.DeepEquals, version.Current)
	c.Assert(result.Results[0].Tools.URL, gc.Equals, "tools:"+version.Current.String())
	c.Assert(result.Results[0].DisableSSLHostnameVerification, jc.IsTrue)
	c.Assert(result.Results[1].Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
	c.Assert(result.Results[2].Error, gc.DeepEquals, apiservertesting.NotFoundError("machine 42"))
}

func (s *toolsSuite) TestToolsError(c *gc.C) {
	getCanRead := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("splat")
	}
	tg := common.NewToolsGetter(s.State, s.State, sprintfURLGetter("%s"), getCanRead)
	args := params.Entities{
		Entities: []params.Entity{{Tag: "machine-42"}},
	}
	result, err := tg.Tools(args)
	c.Assert(err, gc.ErrorMatches, "splat")
	c.Assert(result.Results, gc.HasLen, 1)
}

func (s *toolsSuite) TestSetTools(c *gc.C) {
	getCanWrite := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return tag == names.NewMachineTag("0") || tag == names.NewMachineTag("42")
		}, nil
	}
	ts := common.NewToolsSetter(s.State, getCanWrite)
	c.Assert(ts, gc.NotNil)

	err := s.machine0.SetAgentVersion(version.Current)
	c.Assert(err, gc.IsNil)

	args := params.EntitiesVersion{
		AgentTools: []params.EntityVersion{{
			Tag: "machine-0",
			Tools: &params.Version{
				Version: version.Current,
			},
		}, {
			Tag: "machine-1",
			Tools: &params.Version{
				Version: version.Current,
			},
		}, {
			Tag: "machine-42",
			Tools: &params.Version{
				Version: version.Current,
			},
		}},
	}
	result, err := ts.SetTools(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result.Results, gc.HasLen, 3)
	c.Assert(result.Results[0].Error, gc.IsNil)
	agentTools, err := s.machine0.AgentTools()
	c.Assert(err, gc.IsNil)
	c.Assert(agentTools.Version, gc.DeepEquals, version.Current)
	c.Assert(result.Results[1].Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
	c.Assert(result.Results[2].Error, gc.DeepEquals, apiservertesting.NotFoundError("machine 42"))
}

func (s *toolsSuite) TestToolsSetError(c *gc.C) {
	getCanWrite := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("splat")
	}
	ts := common.NewToolsSetter(s.State, getCanWrite)
	args := params.EntitiesVersion{
		AgentTools: []params.EntityVersion{{
			Tag: "machine-42",
			Tools: &params.Version{
				Version: version.Current,
			},
		}},
	}
	result, err := ts.SetTools(args)
	c.Assert(err, gc.ErrorMatches, "splat")
	c.Assert(result.Results, gc.HasLen, 1)
}

func (s *toolsSuite) TestFindTools(c *gc.C) {
	list := coretools.List{&coretools.Tools{Version: version.Current}}
	s.PatchValue(common.EnvtoolsFindTools, func(g environs.ConfigGetter, major, minor int, filter coretools.Filter, allowRetry bool) (coretools.List, error) {
		c.Assert(major, gc.Equals, 123)
		c.Assert(minor, gc.Equals, 456)
		c.Assert(filter.Number, gc.Equals, version.Current.Number)
		c.Assert(filter.Series, gc.Equals, "win81")
		c.Assert(filter.Arch, gc.Equals, "alpha")
		return list, nil
	})
	toolsFinder := common.NewToolsFinder(s.State, sprintfURLGetter("tools:%s"))
	result, err := toolsFinder.FindTools(params.FindToolsParams{
		Number:       version.Current.Number,
		MajorVersion: 123,
		MinorVersion: 456,
		Series:       "win81",
		Arch:         "alpha",
	})
	c.Assert(err, gc.IsNil)
	c.Assert(result.List, gc.DeepEquals, list)
	c.Assert(result.List[0].URL, gc.Equals, "tools:"+version.Current.String())
}

func (s *toolsSuite) TestFindToolsNotFound(c *gc.C) {
	s.PatchValue(common.EnvtoolsFindTools, func(g environs.ConfigGetter, major, minor int, filter coretools.Filter, allowRetry bool) (list coretools.List, err error) {
		return nil, errors.NotFoundf("tools")
	})
	toolsFinder := common.NewToolsFinder(s.State, sprintfURLGetter("%s"))
	result, err := toolsFinder.FindTools(params.FindToolsParams{})
	c.Assert(err, gc.IsNil)
	c.Assert(result.Error, jc.Satisfies, params.IsCodeNotFound)
}

func (s *toolsSuite) TestToolsURLGetterNoAPIHostPorts(c *gc.C) {
	g := common.NewToolsURLGetter("my-uuid", mockAPIHostPortsGetter{})
	_, err := g.ToolsURL(version.Current)
	c.Assert(err, gc.ErrorMatches, "no API host ports")
}

func (s *toolsSuite) TestToolsURLGetterAPIHostPortsError(c *gc.C) {
	g := common.NewToolsURLGetter("my-uuid", mockAPIHostPortsGetter{err: errors.New("oh noes")})
	_, err := g.ToolsURL(version.Current)
	c.Assert(err, gc.ErrorMatches, "oh noes")
}

func (s *toolsSuite) TestToolsURLGetter(c *gc.C) {
	g := common.NewToolsURLGetter("my-uuid", mockAPIHostPortsGetter{
		hostPorts: [][]network.HostPort{
			network.AddressesWithPort(
				network.NewAddresses("0.1.2.3"),
				1234,
			),
		},
	})
	url, err := g.ToolsURL(version.Current)
	c.Assert(err, gc.IsNil)
	c.Assert(url, gc.Equals, "https://0.1.2.3:1234/environment/my-uuid/tools/"+version.Current.String())
}

type sprintfURLGetter string

func (s sprintfURLGetter) ToolsURL(v version.Binary) (string, error) {
	return fmt.Sprintf(string(s), v), nil
}

type mockAPIHostPortsGetter struct {
	hostPorts [][]network.HostPort
	err       error
}

func (g mockAPIHostPortsGetter) APIHostPorts() ([][]network.HostPort, error) {
	return g.hostPorts, g.err
}
