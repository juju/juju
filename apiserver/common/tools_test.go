// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3/arch"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/network"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/environs"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/binarystorage"
	"github.com/juju/juju/state/stateenvirons"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	jujuversion "github.com/juju/juju/version"
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
	c.Assert(err, jc.ErrorIsNil)
	s.AddDefaultToolsToState(c)
}

func (s *toolsSuite) TestTools(c *gc.C) {
	getCanRead := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return tag == names.NewMachineTag("0") || tag == names.NewMachineTag("42")
		}, nil
	}
	newEnviron := func() (environs.BootstrapEnviron, error) {
		return s.Environ, nil
	}
	tg := common.NewToolsGetter(
		s.State, stateenvirons.EnvironConfigGetter{Model: s.Model}, s.State,
		sprintfURLGetter("tools:%s"), getCanRead, newEnviron,
	)
	c.Assert(tg, gc.NotNil)

	current := coretesting.CurrentVersion(c)
	err := s.machine0.SetAgentVersion(current)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: "machine-0"},
			{Tag: "machine-1"},
			{Tag: "machine-42"},
		}}
	result, err := tg.Tools(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 3)
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(result.Results[0].ToolsList, gc.HasLen, 1)
	tools := result.Results[0].ToolsList[0]
	c.Assert(tools.Version, gc.DeepEquals, current)
	c.Assert(tools.URL, gc.Equals, "tools:"+current.String())
	c.Assert(result.Results[1].Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
	c.Assert(result.Results[2].Error, gc.DeepEquals, apiservertesting.NotFoundError("machine 42"))
}

func (s *toolsSuite) TestSeriesTools(c *gc.C) {
	getCanRead := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return tag == names.NewMachineTag("0")
		}, nil
	}
	newEnviron := func() (environs.BootstrapEnviron, error) {
		return s.Environ, nil
	}
	tg := common.NewToolsGetter(
		s.State, stateenvirons.EnvironConfigGetter{Model: s.Model}, s.State,
		sprintfURLGetter("tools:%s"), getCanRead, newEnviron,
	)
	c.Assert(tg, gc.NotNil)

	current := coretesting.CurrentVersion(c)
	currentCopy := current
	currentCopy.Release = coretesting.HostSeries(c)
	err := s.machine0.SetAgentVersion(currentCopy)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: "machine-0"},
		}}
	result, err := tg.Tools(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(result.Results[0].ToolsList, gc.HasLen, 1)
	tools := result.Results[0].ToolsList[0]
	c.Assert(tools.Version, gc.DeepEquals, current)
	c.Assert(tools.URL, gc.Equals, "tools:"+current.String())
}

func (s *toolsSuite) TestToolsError(c *gc.C) {
	getCanRead := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("splat")
	}
	newEnviron := func() (environs.BootstrapEnviron, error) {
		return s.Environ, nil
	}
	tg := common.NewToolsGetter(
		s.State, stateenvirons.EnvironConfigGetter{Model: s.Model}, s.State,
		sprintfURLGetter("%s"), getCanRead, newEnviron,
	)
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

	current := coretesting.CurrentVersion(c)
	err := s.machine0.SetAgentVersion(current)
	c.Assert(err, jc.ErrorIsNil)

	args := params.EntitiesVersion{
		AgentTools: []params.EntityVersion{{
			Tag: "machine-0",
			Tools: &params.Version{
				Version: current,
			},
		}, {
			Tag: "machine-1",
			Tools: &params.Version{
				Version: current,
			},
		}, {
			Tag: "machine-42",
			Tools: &params.Version{
				Version: current,
			},
		}},
	}
	result, err := ts.SetTools(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 3)
	c.Assert(result.Results[0].Error, gc.IsNil)
	agentTools, err := s.machine0.AgentTools()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(agentTools.Version, gc.DeepEquals, current)
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
				Version: coretesting.CurrentVersion(c),
			},
		}},
	}
	result, err := ts.SetTools(args)
	c.Assert(err, gc.ErrorMatches, "splat")
	c.Assert(result.Results, gc.HasLen, 1)
}

func (s *toolsSuite) TestFindTools(c *gc.C) {
	for i, test := range []struct {
		agentStreamRequested string
		agentStreamsUsed     []string
	}{{
		agentStreamsUsed: []string{"released"},
	}, {
		agentStreamRequested: "pretend",
		agentStreamsUsed:     []string{"pretend"},
	}} {
		c.Logf("test %d", i)
		envtoolsList := coretools.List{
			&coretools.Tools{
				Version: version.MustParseBinary("123.456.0-windows-alpha"),
				Size:    2048,
				SHA256:  "badf00d",
			},
			&coretools.Tools{
				Version: version.MustParseBinary("123.456.1-windows-alpha"),
			},
		}
		storageMetadata := []binarystorage.Metadata{{
			Version: "123.456.0-windows-alpha",
			Size:    1024,
			SHA256:  "feedface",
		}}

		s.PatchValue(common.EnvtoolsFindTools, func(_ envtools.SimplestreamsFetcher, e environs.BootstrapEnviron, major, minor int, streams []string, filter coretools.Filter) (coretools.List, error) {
			c.Assert(major, gc.Equals, 123)
			c.Assert(minor, gc.Equals, 456)
			c.Assert(streams, gc.DeepEquals, test.agentStreamsUsed)
			c.Assert(filter.OSType, gc.Equals, "windows")
			c.Assert(filter.Arch, gc.Equals, "alpha")
			return envtoolsList, nil
		})
		newEnviron := func() (environs.BootstrapEnviron, error) {
			return s.Environ, nil
		}
		toolsFinder := common.NewToolsFinder(
			stateenvirons.EnvironConfigGetter{Model: s.Model}, &mockToolsStorage{metadata: storageMetadata}, sprintfURLGetter("tools:%s"), newEnviron,
		)
		result, err := toolsFinder.FindTools(params.FindToolsParams{
			MajorVersion: 123,
			MinorVersion: 456,
			OSType:       "windows",
			Arch:         "alpha",
			AgentStream:  test.agentStreamRequested,
		})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(result.Error, gc.IsNil)
		c.Check(result.List, jc.DeepEquals, coretools.List{
			&coretools.Tools{
				Version: version.MustParseBinary(storageMetadata[0].Version),
				Size:    storageMetadata[0].Size,
				SHA256:  storageMetadata[0].SHA256,
				URL:     "tools:" + storageMetadata[0].Version,
			},
			&coretools.Tools{
				Version: version.MustParseBinary("123.456.1-windows-alpha"),
				URL:     "tools:123.456.1-windows-alpha",
			},
		})
	}
}

// TODO(juju4) - remove
func (s *toolsSuite) TestFindToolsOldAgent(c *gc.C) {
	for i, test := range []struct {
		agentStreamRequested string
		agentStreamsUsed     []string
	}{{
		agentStreamsUsed: []string{"released"},
	}, {
		agentStreamRequested: "pretend",
		agentStreamsUsed:     []string{"pretend"},
	}} {
		c.Logf("test %d", i)
		envtoolsList := coretools.List{
			&coretools.Tools{
				Version: version.MustParseBinary("2.8.9-focal-amd64"),
				Size:    2048,
				SHA256:  "badf00d",
			},
		}
		storageMetadata := []binarystorage.Metadata{{
			Version: "2.8.9-win2012-amd64",
			Size:    1024,
			SHA256:  "feedface",
		}}

		s.PatchValue(common.EnvtoolsFindTools, func(_ envtools.SimplestreamsFetcher, e environs.BootstrapEnviron, major, minor int, streams []string, filter coretools.Filter) (coretools.List, error) {
			c.Assert(major, gc.Equals, 2)
			c.Assert(minor, gc.Equals, 8)
			c.Assert(streams, gc.DeepEquals, test.agentStreamsUsed)
			c.Assert(filter.OSType, gc.Equals, "ubuntu")
			c.Assert(filter.Arch, gc.Equals, "amd64")
			return envtoolsList, nil
		})
		newEnviron := func() (environs.BootstrapEnviron, error) {
			return s.Environ, nil
		}
		toolsFinder := common.NewToolsFinder(
			stateenvirons.EnvironConfigGetter{Model: s.Model}, &mockToolsStorage{metadata: storageMetadata}, sprintfURLGetter("tools:%s"), newEnviron,
		)
		result, err := toolsFinder.FindTools(params.FindToolsParams{
			Number:       version.MustParse("2.8.9"),
			MajorVersion: 2,
			MinorVersion: 8,
			OSType:       "ubuntu",
			Arch:         "amd64",
			AgentStream:  test.agentStreamRequested,
		})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(result.Error, gc.IsNil)
		c.Check(result.List, jc.DeepEquals, coretools.List{
			&coretools.Tools{
				Version: version.MustParseBinary("2.8.9-ubuntu-amd64"),
				Size:    2048,
				SHA256:  "badf00d",
				URL:     "tools:2.8.9-ubuntu-amd64",
			},
		})
	}
}

func (s *toolsSuite) TestFindToolsNotFound(c *gc.C) {
	s.PatchValue(common.EnvtoolsFindTools, func(_ envtools.SimplestreamsFetcher, e environs.BootstrapEnviron, major, minor int, stream []string, filter coretools.Filter) (list coretools.List, err error) {
		return nil, errors.NotFoundf("tools")
	})
	newEnviron := func() (environs.BootstrapEnviron, error) {
		return s.Environ, nil
	}
	toolsFinder := common.NewToolsFinder(stateenvirons.EnvironConfigGetter{Model: s.Model}, s.State, sprintfURLGetter("%s"), newEnviron)
	result, err := toolsFinder.FindTools(params.FindToolsParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, jc.Satisfies, params.IsCodeNotFound)
}

func (s *toolsSuite) TestFindToolsExactInStorage(c *gc.C) {
	mockToolsStorage := &mockToolsStorage{
		metadata: []binarystorage.Metadata{
			{Version: "1.22-beta1-ubuntu-amd64"},
			{Version: "1.22.0-ubuntu-amd64"},
		},
	}

	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })
	s.PatchValue(&coreos.HostOS, func() coreos.OSType { return coreos.Ubuntu })
	s.PatchValue(&jujuversion.Current, version.MustParseBinary("1.22-beta1-ubuntu-amd64").Number)
	s.testFindToolsExact(c, mockToolsStorage, true, true)
	s.PatchValue(&jujuversion.Current, version.MustParseBinary("1.22.0-ubuntu-amd64").Number)
	s.testFindToolsExact(c, mockToolsStorage, true, false)
}

func (s *toolsSuite) TestFindToolsExactNotInStorage(c *gc.C) {
	mockToolsStorage := &mockToolsStorage{}
	s.PatchValue(&jujuversion.Current, version.MustParse("1.22-beta1"))
	s.testFindToolsExact(c, mockToolsStorage, false, true)
	s.PatchValue(&jujuversion.Current, version.MustParse("1.22.0"))
	s.testFindToolsExact(c, mockToolsStorage, false, false)
}

func (s *toolsSuite) testFindToolsExact(c *gc.C, t common.ToolsStorageGetter, inStorage bool, develVersion bool) {
	var called bool
	current := coretesting.CurrentVersion(c)
	s.PatchValue(common.EnvtoolsFindTools, func(_ envtools.SimplestreamsFetcher, e environs.BootstrapEnviron, major, minor int, stream []string, filter coretools.Filter) (list coretools.List, err error) {
		called = true
		c.Assert(filter.Number, gc.Equals, jujuversion.Current)
		c.Assert(filter.OSType, gc.Equals, current.Release)
		c.Assert(filter.Arch, gc.Equals, arch.HostArch())
		if develVersion {
			c.Assert(stream, gc.DeepEquals, []string{"devel", "proposed", "released"})
		} else {
			c.Assert(stream, gc.DeepEquals, []string{"released"})
		}
		return nil, errors.NotFoundf("tools")
	})
	newEnviron := func() (environs.BootstrapEnviron, error) {
		return s.Environ, nil
	}
	toolsFinder := common.NewToolsFinder(stateenvirons.EnvironConfigGetter{Model: s.Model}, t, sprintfURLGetter("tools:%s"), newEnviron)
	result, err := toolsFinder.FindTools(params.FindToolsParams{
		Number:       jujuversion.Current,
		MajorVersion: -1,
		MinorVersion: -1,
		OSType:       current.Release,
		Arch:         arch.HostArch(),
	})
	c.Assert(err, jc.ErrorIsNil)
	if inStorage {
		c.Assert(result.Error, gc.IsNil)
		c.Assert(called, jc.IsFalse)
	} else {
		c.Assert(result.Error, gc.ErrorMatches, "tools not found")
		c.Assert(called, jc.IsTrue)
	}
}

func (s *toolsSuite) TestFindToolsToolsStorageError(c *gc.C) {
	var called bool
	s.PatchValue(common.EnvtoolsFindTools, func(_ envtools.SimplestreamsFetcher, e environs.BootstrapEnviron, major, minor int, stream []string, filter coretools.Filter) (list coretools.List, err error) {
		called = true
		return nil, errors.NotFoundf("tools")
	})
	newEnviron := func() (environs.BootstrapEnviron, error) {
		return s.Environ, nil
	}
	toolsFinder := common.NewToolsFinder(stateenvirons.EnvironConfigGetter{Model: s.Model}, &mockToolsStorage{
		err: errors.New("AllMetadata failed"),
	}, sprintfURLGetter("tools:%s"), newEnviron)
	result, err := toolsFinder.FindTools(params.FindToolsParams{
		MajorVersion: 1,
		MinorVersion: -1,
	})
	c.Assert(err, jc.ErrorIsNil)
	// ToolsStorage errors always cause FindTools to bail. Only
	// if AllMetadata succeeds but returns nothing that matches
	// do we continue on to searching simplestreams.
	c.Assert(result.Error, gc.ErrorMatches, "AllMetadata failed")
	c.Assert(called, jc.IsFalse)
}

func (s *toolsSuite) TestToolsURLGetterNoAPIHostPorts(c *gc.C) {
	g := common.NewToolsURLGetter("my-uuid", mockAPIHostPortsGetter{})
	_, err := g.ToolsURLs(coretesting.CurrentVersion(c))
	c.Assert(err, gc.ErrorMatches, "no suitable API server address to pick from")
}

func (s *toolsSuite) TestToolsURLGetterAPIHostPortsError(c *gc.C) {
	g := common.NewToolsURLGetter("my-uuid", mockAPIHostPortsGetter{err: errors.New("oh noes")})
	_, err := g.ToolsURLs(coretesting.CurrentVersion(c))
	c.Assert(err, gc.ErrorMatches, "oh noes")
}

func (s *toolsSuite) TestToolsURLGetter(c *gc.C) {
	g := common.NewToolsURLGetter("my-uuid", mockAPIHostPortsGetter{
		hostPorts: []network.SpaceHostPorts{
			network.NewSpaceHostPorts(1234, "0.1.2.3"),
		},
	})
	current := coretesting.CurrentVersion(c)
	urls, err := g.ToolsURLs(current)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(urls, jc.DeepEquals, []string{
		"https://0.1.2.3:1234/model/my-uuid/tools/" + current.String(),
	})
}

type sprintfURLGetter string

func (s sprintfURLGetter) ToolsURLs(v version.Binary) ([]string, error) {
	return []string{fmt.Sprintf(string(s), v)}, nil
}

type mockAPIHostPortsGetter struct {
	hostPorts []network.SpaceHostPorts
	err       error
}

func (g mockAPIHostPortsGetter) APIHostPortsForAgents() ([]network.SpaceHostPorts, error) {
	return g.hostPorts, g.err
}

type mockToolsStorage struct {
	binarystorage.Storage
	metadata []binarystorage.Metadata
	err      error
}

func (s *mockToolsStorage) ToolsStorage() (binarystorage.StorageCloser, error) {
	return s, nil
}

func (s *mockToolsStorage) Close() error {
	return nil
}

func (s *mockToolsStorage) AllMetadata() ([]binarystorage.Metadata, error) {
	return s.metadata, s.err
}
