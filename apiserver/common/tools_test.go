// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/juju/arch"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/toolstorage"
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
	c.Assert(err, jc.ErrorIsNil)
	s.AddDefaultToolsToState(c)
}

func (s *toolsSuite) TestTools(c *gc.C) {
	getCanRead := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return tag == names.NewMachineTag("0") || tag == names.NewMachineTag("42")
		}, nil
	}
	tg := common.NewToolsGetter(s.State, s.State, s.State, sprintfURLGetter("tools:%s"), getCanRead)
	c.Assert(tg, gc.NotNil)

	err := s.machine0.SetAgentVersion(version.Current)
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
	tg := common.NewToolsGetter(s.State, s.State, s.State, sprintfURLGetter("%s"), getCanRead)
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
	c.Assert(err, jc.ErrorIsNil)

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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 3)
	c.Assert(result.Results[0].Error, gc.IsNil)
	agentTools, err := s.machine0.AgentTools()
	c.Assert(err, jc.ErrorIsNil)
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
	envtoolsList := coretools.List{
		&coretools.Tools{
			Version: version.MustParseBinary("123.456.0-win81-alpha"),
			Size:    2048,
			SHA256:  "badf00d",
		},
		&coretools.Tools{
			Version: version.MustParseBinary("123.456.1-win81-alpha"),
		},
	}
	storageMetadata := []toolstorage.Metadata{{
		Version: version.MustParseBinary("123.456.0-win81-alpha"),
		Size:    1024,
		SHA256:  "feedface",
	}}

	s.PatchValue(common.EnvtoolsFindTools, func(e environs.Environ, major, minor int, stream string, filter coretools.Filter) (coretools.List, error) {
		c.Assert(major, gc.Equals, 123)
		c.Assert(minor, gc.Equals, 456)
		c.Assert(stream, gc.Equals, "released")
		c.Assert(filter.Series, gc.Equals, "win81")
		c.Assert(filter.Arch, gc.Equals, "alpha")
		return envtoolsList, nil
	})
	toolsFinder := common.NewToolsFinder(s.State, &mockToolsStorage{metadata: storageMetadata}, sprintfURLGetter("tools:%s"))
	result, err := toolsFinder.FindTools(params.FindToolsParams{
		MajorVersion: 123,
		MinorVersion: 456,
		Series:       "win81",
		Arch:         "alpha",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.List, gc.DeepEquals, coretools.List{
		&coretools.Tools{
			Version: storageMetadata[0].Version,
			Size:    storageMetadata[0].Size,
			SHA256:  storageMetadata[0].SHA256,
			URL:     "tools:" + storageMetadata[0].Version.String(),
		},
		envtoolsList[1],
	})
}

func (s *toolsSuite) TestFindToolsNotFound(c *gc.C) {
	s.PatchValue(common.EnvtoolsFindTools, func(e environs.Environ, major, minor int, stream string, filter coretools.Filter) (list coretools.List, err error) {
		return nil, errors.NotFoundf("tools")
	})
	toolsFinder := common.NewToolsFinder(s.State, s.State, sprintfURLGetter("%s"))
	result, err := toolsFinder.FindTools(params.FindToolsParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, jc.Satisfies, params.IsCodeNotFound)
}

func (s *toolsSuite) TestFindToolsExactInStorage(c *gc.C) {
	mockToolsStorage := &mockToolsStorage{
		metadata: []toolstorage.Metadata{
			{Version: version.MustParseBinary("1.22-beta1-trusty-amd64")},
			{Version: version.MustParseBinary("1.22.0-trusty-amd64")},
		},
	}

	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })
	s.PatchValue(&version.Current, version.MustParseBinary("1.22-beta1-trusty-amd64"))
	s.testFindToolsExact(c, mockToolsStorage, true, true)
	s.PatchValue(&version.Current, version.MustParseBinary("1.22.0-trusty-amd64"))
	s.testFindToolsExact(c, mockToolsStorage, true, false)
}

func (s *toolsSuite) TestFindToolsExactNotInStorage(c *gc.C) {
	mockToolsStorage := &mockToolsStorage{}
	s.PatchValue(&version.Current.Number, version.MustParse("1.22-beta1"))
	s.testFindToolsExact(c, mockToolsStorage, false, true)
	s.PatchValue(&version.Current.Number, version.MustParse("1.22.0"))
	s.testFindToolsExact(c, mockToolsStorage, false, false)
}

func (s *toolsSuite) testFindToolsExact(c *gc.C, t common.ToolsStorageGetter, inStorage bool, develVersion bool) {
	var called bool
	s.PatchValue(common.EnvtoolsFindTools, func(e environs.Environ, major, minor int, stream string, filter coretools.Filter) (list coretools.List, err error) {
		called = true
		c.Assert(filter.Number, gc.Equals, version.Current.Number)
		c.Assert(filter.Series, gc.Equals, version.Current.Series)
		c.Assert(filter.Arch, gc.Equals, arch.HostArch())
		if develVersion {
			c.Assert(stream, gc.Equals, "devel")
		} else {
			c.Assert(stream, gc.Equals, "released")
		}
		return nil, errors.NotFoundf("tools")
	})
	toolsFinder := common.NewToolsFinder(s.State, t, sprintfURLGetter("tools:%s"))
	result, err := toolsFinder.FindTools(params.FindToolsParams{
		Number:       version.Current.Number,
		MajorVersion: -1,
		MinorVersion: -1,
		Series:       version.Current.Series,
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
	s.PatchValue(common.EnvtoolsFindTools, func(e environs.Environ, major, minor int, stream string, filter coretools.Filter) (list coretools.List, err error) {
		called = true
		return nil, errors.NotFoundf("tools")
	})
	toolsFinder := common.NewToolsFinder(s.State, &mockToolsStorage{
		err: errors.New("AllMetadata failed"),
	}, sprintfURLGetter("tools:%s"))
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
			network.NewHostPorts(1234, "0.1.2.3"),
		},
	})
	url, err := g.ToolsURL(version.Current)
	c.Assert(err, jc.ErrorIsNil)
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

type mockToolsStorage struct {
	toolstorage.Storage
	metadata []toolstorage.Metadata
	err      error
}

func (s *mockToolsStorage) ToolsStorage() (toolstorage.StorageCloser, error) {
	return s, nil
}

func (s *mockToolsStorage) Close() error {
	return nil
}

func (s *mockToolsStorage) AllMetadata() ([]toolstorage.Metadata, error) {
	return s.metadata, s.err
}
