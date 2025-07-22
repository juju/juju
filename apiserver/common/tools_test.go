// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/os/v2"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	coreagentbinary "github.com/juju/juju/core/agentbinary"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/agentbinary"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/rpc/params"
)

type getToolsSuite struct {
	modelAgentService *mocks.MockModelAgentService
	toolsFinder       *mocks.MockToolsFinder
	store             *mocks.MockObjectStore
}

func TestGetToolsSuite(t *testing.T) {
	tc.Run(t, &getToolsSuite{})
}

func (s *getToolsSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelAgentService = mocks.NewMockModelAgentService(ctrl)
	s.toolsFinder = mocks.NewMockToolsFinder(ctrl)
	s.store = mocks.NewMockObjectStore(ctrl)

	return ctrl
}

func (s *getToolsSuite) TestTools(c *tc.C) {
	defer s.setup(c).Finish()

	getCanRead := func(ctx context.Context) (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return tag == names.NewMachineTag("0") || tag == names.NewMachineTag("42")
		}, nil
	}
	tg := common.NewToolsGetter(
		s.modelAgentService, nil, s.toolsFinder, getCanRead,
	)
	c.Assert(tg, tc.NotNil)

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: "machine-0"},
			{Tag: "machine-1"},
			{Tag: "machine-42"},
		},
	}

	agentBinary := coreagentbinary.Version{
		Number: jujuversion.Current,
		Arch:   arch.HostArch(),
	}
	s.modelAgentService.EXPECT().GetMachineTargetAgentVersion(gomock.Any(), machine.Name("0")).Return(agentBinary, nil)
	s.modelAgentService.EXPECT().GetMachineTargetAgentVersion(gomock.Any(), machine.Name("42")).
		Return(coreagentbinary.Version{}, machineerrors.MachineNotFound)

	current := coretesting.CurrentVersion()
	s.toolsFinder.EXPECT().FindAgents(gomock.Any(), common.FindAgentsParams{
		Number: current.Number,
		OSType: os.Ubuntu.String(),
		Arch:   current.Arch,
	}).Return(coretools.List{{
		Version: current,
		URL:     "tools:" + current.String(),
	}}, nil)

	result, err := tg.Tools(c.Context(), args)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 3)
	c.Assert(result.Results[0].Error, tc.IsNil)
	c.Assert(result.Results[0].ToolsList, tc.HasLen, 1)
	tools := result.Results[0].ToolsList[0]
	c.Assert(tools.Version, tc.DeepEquals, current)
	c.Assert(tools.URL, tc.Equals, "tools:"+current.String())
	c.Assert(result.Results[1].Error, tc.DeepEquals, apiservertesting.ErrUnauthorized)
	c.Assert(result.Results[2].Error, tc.DeepEquals, apiservertesting.NotFoundError(`"machine 42"`))
}

func (s *getToolsSuite) TestToolsError(c *tc.C) {
	defer s.setup(c).Finish()

	getCanRead := func(ctx context.Context) (common.AuthFunc, error) {
		return nil, fmt.Errorf("splat")
	}
	tg := common.NewToolsGetter(
		s.modelAgentService, nil, s.toolsFinder, getCanRead,
	)
	c.Assert(tg, tc.NotNil)

	args := params.Entities{
		Entities: []params.Entity{{Tag: "machine-42"}},
	}
	result, err := tg.Tools(c.Context(), args)
	c.Assert(err, tc.ErrorMatches, "splat")
	c.Assert(result.Results, tc.HasLen, 1)
}

type findToolsSuite struct {
	testhelpers.IsolationSuite

	urlGetter *mocks.MockToolsURLGetter
	store     *mocks.MockObjectStore

	mockAgentBinaryService *mocks.MockAgentBinaryService
}

func TestFindToolsSuite(t *testing.T) {
	tc.Run(t, &findToolsSuite{})
}

func (s *findToolsSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
}

func (s *findToolsSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.urlGetter = mocks.NewMockToolsURLGetter(ctrl)
	s.urlGetter.EXPECT().ToolsURLs(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, arg semversion.Binary) ([]string, error) {
		return []string{fmt.Sprintf("tools:%v", arg)}, nil
	}).AnyTimes()

	s.mockAgentBinaryService = mocks.NewMockAgentBinaryService(ctrl)
	s.store = mocks.NewMockObjectStore(ctrl)
	return ctrl
}

func (s *findToolsSuite) TestFindToolsMatchMajor(c *tc.C) {
	defer s.setup(c).Finish()

	envtoolsList := coretools.List{
		&coretools.Tools{
			Version: semversion.MustParseBinary("123.456.0-ubuntu-alpha"),
			Size:    2048,
			SHA256:  "badf00d",
		},
		&coretools.Tools{
			Version: semversion.MustParseBinary("123.456.1-ubuntu-alpha"),
		},
	}

	s.mockAgentBinaryService.EXPECT().GetEnvironAgentBinariesFinder().Return(
		func(_ context.Context, major, minor int, version semversion.Number, _ string, filter coretools.Filter) (coretools.List, error) {
			c.Assert(major, tc.Equals, 123)
			c.Assert(minor, tc.Equals, 456)
			c.Assert(filter.OSType, tc.Equals, "ubuntu")
			c.Assert(filter.Arch, tc.Equals, "alpha")
			return envtoolsList, nil
		},
	)
	storageMetadata := []agentbinary.Metadata{{
		Version: "123.456.0",
		Size:    1024,
		Arch:    "alpha",
		SHA256:  "feedface",
	}, {
		Version: "666.456.0",
		Size:    1024,
		Arch:    "alpha",
		SHA256:  "feedface666",
	}}
	s.mockAgentBinaryService.EXPECT().ListAgentBinaries(gomock.Any()).Return(storageMetadata, nil)

	toolsFinder := common.NewToolsFinder(s.urlGetter, s.store, s.mockAgentBinaryService)

	result, err := toolsFinder.FindAgents(c.Context(), common.FindAgentsParams{
		MajorVersion: 123,
		MinorVersion: 456,
		OSType:       "ubuntu",
		Arch:         "alpha",
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, coretools.List{
		&coretools.Tools{
			Version: semversion.MustParseBinary("123.456.0-ubuntu-alpha"),
			Size:    1024,
			SHA256:  "feedface",
			URL:     "tools:123.456.0-ubuntu-alpha",
		},
		&coretools.Tools{
			Version: semversion.MustParseBinary("123.456.1-ubuntu-alpha"),
			URL:     "tools:123.456.1-ubuntu-alpha",
		},
	})
}

func (s *findToolsSuite) TestFindToolsRequestAgentStream(c *tc.C) {
	defer s.setup(c).Finish()

	envtoolsList := coretools.List{
		&coretools.Tools{
			Version: semversion.MustParseBinary("123.456.0-ubuntu-alpha"),
			Size:    2048,
			SHA256:  "badf00d",
		},
		&coretools.Tools{
			Version: semversion.MustParseBinary("123.456.1-ubuntu-alpha"),
		},
	}

	s.mockAgentBinaryService.EXPECT().GetEnvironAgentBinariesFinder().Return(
		func(_ context.Context, major, minor int, version semversion.Number, requestedStream string, filter coretools.Filter) (coretools.List, error) {
			c.Assert(major, tc.Equals, 123)
			c.Assert(minor, tc.Equals, 456)
			c.Assert(requestedStream, tc.Equals, "pretend")
			c.Assert(filter.OSType, tc.Equals, "ubuntu")
			c.Assert(filter.Arch, tc.Equals, "alpha")
			return envtoolsList, nil
		},
	)

	storageMetadata := []agentbinary.Metadata{{
		Version: "123.456.0",
		Size:    1024,
		Arch:    "alpha",
		SHA256:  "feedface",
	}}
	s.mockAgentBinaryService.EXPECT().ListAgentBinaries(gomock.Any()).Return(storageMetadata, nil)

	toolsFinder := common.NewToolsFinder(s.urlGetter, s.store, s.mockAgentBinaryService)
	result, err := toolsFinder.FindAgents(c.Context(), common.FindAgentsParams{
		MajorVersion: 123,
		MinorVersion: 456,
		OSType:       "ubuntu",
		Arch:         "alpha",
		AgentStream:  "pretend",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, coretools.List{
		&coretools.Tools{
			Version: semversion.MustParseBinary("123.456.0-ubuntu-alpha"),
			Size:    1024,
			SHA256:  "feedface",
			URL:     "tools:123.456.0-ubuntu-alpha",
		},
		&coretools.Tools{
			Version: semversion.MustParseBinary("123.456.1-ubuntu-alpha"),
			URL:     "tools:123.456.1-ubuntu-alpha",
		},
	})
}

func (s *findToolsSuite) TestFindToolsNotFound(c *tc.C) {
	defer s.setup(c).Finish()

	s.mockAgentBinaryService.EXPECT().GetEnvironAgentBinariesFinder().Return(
		func(_ context.Context, major, minor int, version semversion.Number, requestedStream string, filter coretools.Filter) (coretools.List, error) {
			return nil, errors.NotFoundf("tools")
		},
	)

	s.mockAgentBinaryService.EXPECT().ListAgentBinaries(gomock.Any()).Return(nil, nil)

	toolsFinder := common.NewToolsFinder(nil, s.store, s.mockAgentBinaryService)
	_, err := toolsFinder.FindAgents(c.Context(), common.FindAgentsParams{})
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *findToolsSuite) TestFindToolsToolsStorageError(c *tc.C) {
	defer s.setup(c).Finish()

	s.mockAgentBinaryService.EXPECT().ListAgentBinaries(gomock.Any()).Return(nil, errors.New("AllMetadata failed"))

	toolsFinder := common.NewToolsFinder(s.urlGetter, s.store, s.mockAgentBinaryService)
	_, err := toolsFinder.FindAgents(c.Context(), common.FindAgentsParams{})
	// ToolsStorage errors always cause FindAgents to bail. Only
	// if AllMetadata succeeds but returns nothing that matches
	// do we continue on to searching simplestreams.
	c.Assert(err, tc.ErrorMatches, "AllMetadata failed")
}

func TestGetUrlSuite(t *testing.T) {
	tc.Run(t, &getUrlSuite{})
}

type getUrlSuite struct {
	apiHostPortsGetter *mocks.MockAPIHostPortsForAgentsGetter
}

func (s *getUrlSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.apiHostPortsGetter = mocks.NewMockAPIHostPortsForAgentsGetter(ctrl)
	return ctrl
}

func (s *getUrlSuite) TestToolsURLGetterNoAPIHostPorts(c *tc.C) {
	defer s.setup(c).Finish()

	s.apiHostPortsGetter.EXPECT().GetAllAPIAddressesForAgents(gomock.Any()).Return(nil, nil)

	g := common.NewToolsURLGetter("my-uuid", s.apiHostPortsGetter)
	_, err := g.ToolsURLs(c.Context(), coretesting.CurrentVersion())
	c.Assert(err, tc.ErrorMatches, "no suitable API server address to pick from")
}

func (s *getUrlSuite) TestToolsURLGetterAPIHostPortsError(c *tc.C) {
	defer s.setup(c).Finish()

	s.apiHostPortsGetter.EXPECT().GetAllAPIAddressesForAgents(gomock.Any()).Return(nil, errors.New("oh noes"))

	g := common.NewToolsURLGetter("my-uuid", s.apiHostPortsGetter)
	_, err := g.ToolsURLs(c.Context(), coretesting.CurrentVersion())
	c.Assert(err, tc.ErrorMatches, "oh noes")
}

func (s *getUrlSuite) TestToolsURLGetter(c *tc.C) {
	defer s.setup(c).Finish()

	addrs := []string{"0.1.2.3:1234"}
	s.apiHostPortsGetter.EXPECT().GetAllAPIAddressesForAgents(gomock.Any()).Return(addrs, nil)

	g := common.NewToolsURLGetter("my-uuid", s.apiHostPortsGetter)
	current := coretesting.CurrentVersion()
	urls, err := g.ToolsURLs(c.Context(), current)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(urls, tc.DeepEquals, []string{
		"https://0.1.2.3:1234/model/my-uuid/tools/" + current.String(),
	})
}
