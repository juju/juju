// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/controller"
	coreagentbinary "github.com/juju/juju/core/agentbinary"
	corearch "github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	jujuversion "github.com/juju/juju/core/version"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	coretesting "github.com/juju/juju/internal/testing"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/rpc/params"
)

type getToolsSuite struct {
	controllerConfigService *mocks.MockControllerConfigService
	modelAgentService       *mocks.MockModelAgentService
	toolsFinder             *mocks.MockToolsFinder
	store                   *mocks.MockObjectStore
}

var _ = gc.Suite(&getToolsSuite{})

func (s *getToolsSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.controllerConfigService = mocks.NewMockControllerConfigService(ctrl)
	s.modelAgentService = mocks.NewMockModelAgentService(ctrl)
	s.toolsFinder = mocks.NewMockToolsFinder(ctrl)
	s.store = mocks.NewMockObjectStore(ctrl)

	return ctrl
}

func (s *getToolsSuite) TestTools(c *gc.C) {
	defer s.setup(c).Finish()

	getCanRead := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return tag == names.NewMachineTag("0") || tag == names.NewMachineTag("42")
		}, nil
	}
	tg := common.NewToolsGetter(
		s.controllerConfigService,
		s.modelAgentService, nil,
		nil, s.toolsFinder, getCanRead,
	)
	c.Assert(tg, gc.NotNil)

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: "machine-0"},
			{Tag: "machine-1"},
			{Tag: "machine-42"},
		},
	}

	cfg := controller.Config{}
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(cfg, nil)

	agentBinary := coreagentbinary.Version{
		Number: jujuversion.Current,
		Arch:   corearch.HostArch(),
	}

	s.modelAgentService.EXPECT().GetMachineTargetAgentVersion(gomock.Any(), machine.Name("0")).Return(agentBinary, nil)
	s.modelAgentService.EXPECT().GetMachineTargetAgentVersion(gomock.Any(), machine.Name("42")).
		Return(coreagentbinary.Version{}, machineerrors.MachineNotFound)

	current := coretesting.CurrentVersion()
	s.toolsFinder.EXPECT().FindAgents(context.Background(), common.FindAgentsParams{
		ControllerCfg: cfg,
		Number:        current.Number,
		OSType:        "ubuntu",
		Arch:          current.Arch,
	}).Return(coretools.List{{
		Version: current,
		URL:     "tools:" + current.String(),
	}}, nil)

	result, err := tg.Tools(context.Background(), args)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 3)
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(result.Results[0].ToolsList, gc.HasLen, 1)
	tools := result.Results[0].ToolsList[0]
	c.Assert(tools.Version, gc.DeepEquals, current)
	c.Assert(tools.URL, gc.Equals, "tools:"+current.String())
	c.Assert(result.Results[1].Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
	c.Assert(result.Results[2].Error, gc.DeepEquals, apiservertesting.NotFoundError(`"machine 42"`))
}

func (s *getToolsSuite) TestToolsError(c *gc.C) {
	defer s.setup(c).Finish()

	getCanRead := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("splat")
	}
	tg := common.NewToolsGetter(
		s.controllerConfigService,
		s.modelAgentService, nil,
		nil, s.toolsFinder, getCanRead,
	)
	c.Assert(tg, gc.NotNil)

	args := params.Entities{
		Entities: []params.Entity{{Tag: "machine-42"}},
	}
	result, err := tg.Tools(context.Background(), args)
	c.Assert(err, gc.ErrorMatches, "splat")
	c.Assert(result.Results, gc.HasLen, 1)
}

var _ = gc.Suite(&getUrlSuite{})

type getUrlSuite struct {
	apiHostPortsGetter *mocks.MockAPIHostPortsForAgentsGetter
}

var _ = gc.Suite(&getUrlSuite{})

func (s *getUrlSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.apiHostPortsGetter = mocks.NewMockAPIHostPortsForAgentsGetter(ctrl)
	return ctrl
}

func (s *getUrlSuite) TestToolsURLGetterNoAPIHostPorts(c *gc.C) {
	defer s.setup(c).Finish()

	s.apiHostPortsGetter.EXPECT().APIHostPortsForAgents(gomock.Any()).Return(nil, nil)

	g := common.NewToolsURLGetter("my-uuid", s.apiHostPortsGetter)
	_, err := g.ToolsURLs(context.Background(), coretesting.FakeControllerConfig(), coretesting.CurrentVersion())
	c.Assert(err, gc.ErrorMatches, "no suitable API server address to pick from")
}

func (s *getUrlSuite) TestToolsURLGetterAPIHostPortsError(c *gc.C) {
	defer s.setup(c).Finish()

	s.apiHostPortsGetter.EXPECT().APIHostPortsForAgents(gomock.Any()).Return(nil, errors.New("oh noes"))

	g := common.NewToolsURLGetter("my-uuid", s.apiHostPortsGetter)
	_, err := g.ToolsURLs(context.Background(), coretesting.FakeControllerConfig(), coretesting.CurrentVersion())
	c.Assert(err, gc.ErrorMatches, "oh noes")
}

func (s *getUrlSuite) TestToolsURLGetter(c *gc.C) {
	defer s.setup(c).Finish()

	s.apiHostPortsGetter.EXPECT().APIHostPortsForAgents(gomock.Any()).Return([]network.SpaceHostPorts{
		network.NewSpaceHostPorts(1234, "0.1.2.3"),
	}, nil)

	g := common.NewToolsURLGetter("my-uuid", s.apiHostPortsGetter)
	current := coretesting.CurrentVersion()
	urls, err := g.ToolsURLs(context.Background(), coretesting.FakeControllerConfig(), current)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(urls, jc.DeepEquals, []string{
		"https://0.1.2.3:1234/model/my-uuid/tools/" + current.String(),
	})
}
