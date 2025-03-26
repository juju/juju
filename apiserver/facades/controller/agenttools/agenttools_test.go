// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agenttools

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/tools"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/internal/version"
	"github.com/juju/juju/state"
)

var _ = gc.Suite(&AgentToolsSuite{})

type AgentToolsSuite struct {
	coretesting.BaseSuite
	modelConfigService *MockModelConfigService
	modelAgentService  *MockModelAgentService
}

type dummyEnviron struct {
	environs.Environ
}

func (s *AgentToolsSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.modelConfigService = NewMockModelConfigService(ctrl)
	s.modelAgentService = NewMockModelAgentService(ctrl)
	return ctrl
}

func (s *AgentToolsSuite) TestCheckTools(c *gc.C) {
	defer s.setupMocks(c).Finish()

	expVer, err := version.Parse("2.5.0")
	c.Assert(err, jc.ErrorIsNil)
	s.modelAgentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(expVer, nil)
	modelConfig, err := config.New(config.NoDefaults, coretesting.FakeConfig())
	c.Assert(err, jc.ErrorIsNil)
	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(modelConfig, nil)

	var (
		calledWithMajor, calledWithMinor int
	)
	fakeToolFinder := func(_ context.Context, _ tools.SimplestreamsFetcher, e environs.BootstrapEnviron, maj int, min int, streams []string, filter coretools.Filter) (coretools.List, error) {
		calledWithMajor = maj
		calledWithMinor = min
		ver := version.Binary{Number: version.Number{Major: maj, Minor: min}}
		t := coretools.Tools{Version: ver, URL: "http://example.com", Size: 1}
		c.Assert(calledWithMajor, gc.Equals, 2)
		c.Assert(calledWithMinor, gc.Equals, 5)
		c.Assert(streams, gc.DeepEquals, []string{"released"})
		return coretools.List{&t}, nil
	}

	api, err := NewAgentToolsAPI(nil, getDummyEnviron, fakeToolFinder, nil, nil, loggertesting.WrapCheckLog(c), s.modelConfigService, s.modelAgentService)
	c.Assert(err, jc.ErrorIsNil)

	obtainedVer, err := api.checkToolsAvailability(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedVer, gc.Equals, expVer)
}

func (s *AgentToolsSuite) TestCheckToolsNonReleasedStream(c *gc.C) {
	defer s.setupMocks(c).Finish()

	expVer, err := version.Parse("2.5-alpha1")
	c.Assert(err, jc.ErrorIsNil)
	s.modelAgentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(expVer, nil)

	sConfig := coretesting.FakeConfig()
	sConfig = sConfig.Merge(coretesting.Attrs{
		"agent-stream": "proposed",
	})
	cfg, err := config.New(config.NoDefaults, sConfig)
	c.Assert(err, jc.ErrorIsNil)
	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(cfg, nil)

	var (
		calledWithMajor, calledWithMinor int
		calledWithStreams                [][]string
	)
	fakeToolFinder := func(_ context.Context, _ tools.SimplestreamsFetcher, e environs.BootstrapEnviron, maj int, min int, streams []string, filter coretools.Filter) (coretools.List, error) {
		calledWithMajor = maj
		calledWithMinor = min
		calledWithStreams = append(calledWithStreams, streams)
		if len(streams) == 1 && streams[0] == "released" {
			return nil, coretools.ErrNoMatches
		}
		ver := version.Binary{Number: version.Number{Major: maj, Minor: min}}
		t := coretools.Tools{Version: ver, URL: "http://example.com", Size: 1}
		c.Assert(calledWithMajor, gc.Equals, 2)
		c.Assert(calledWithMinor, gc.Equals, 5)
		return coretools.List{&t}, nil
	}

	api, err := NewAgentToolsAPI(nil, getDummyEnviron, fakeToolFinder, nil, nil, loggertesting.WrapCheckLog(c), s.modelConfigService, s.modelAgentService)
	c.Assert(err, jc.ErrorIsNil)

	obtainedVer, err := api.checkToolsAvailability(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(calledWithStreams, gc.DeepEquals, [][]string{{"proposed", "released"}})
	c.Assert(obtainedVer, gc.Equals, version.Number{Major: 2, Minor: 5, Patch: 0})
}

type mockState struct{}

func (e *mockState) Model() (*state.Model, error) {
	return &state.Model{}, nil
}

func (s *AgentToolsSuite) TestUpdateToolsAvailability(c *gc.C) {
	defer s.setupMocks(c).Finish()

	expVer, err := version.Parse("2.5.0")
	c.Assert(err, jc.ErrorIsNil)
	s.modelAgentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(expVer, nil)
	modelConfig, err := config.New(config.NoDefaults, coretesting.FakeConfig())
	c.Assert(err, jc.ErrorIsNil)
	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(modelConfig, nil)

	fakeToolFinder := func(_ context.Context, _ tools.SimplestreamsFetcher, _ environs.BootstrapEnviron, _ int, _ int, _ []string, _ coretools.Filter) (coretools.List, error) {
		ver := version.Binary{Number: version.Number{Major: 2, Minor: 5, Patch: 2}}
		olderVer := version.Binary{Number: version.Number{Major: 2, Minor: 5, Patch: 1}}
		t := coretools.Tools{Version: ver, URL: "http://example.com", Size: 1}
		tOld := coretools.Tools{Version: olderVer, URL: "http://example.com", Size: 1}
		return coretools.List{&t, &tOld}, nil
	}

	var ver version.Number
	fakeUpdate := func(_ *state.Model, v version.Number) error {
		ver = v
		return nil
	}

	api, err := NewAgentToolsAPI(&mockState{}, getDummyEnviron, fakeToolFinder, fakeUpdate, nil, loggertesting.WrapCheckLog(c), s.modelConfigService, s.modelAgentService)
	c.Assert(err, jc.ErrorIsNil)

	err = api.updateToolsAvailability(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ver, gc.Equals, version.Number{Major: 2, Minor: 5, Patch: 2})
}

func (s *AgentToolsSuite) TestUpdateToolsAvailabilityNoMatches(c *gc.C) {
	defer s.setupMocks(c).Finish()

	expVer, err := version.Parse("2.5.0")
	c.Assert(err, jc.ErrorIsNil)
	s.modelAgentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(expVer, nil)
	modelConfig, err := config.New(config.NoDefaults, coretesting.FakeConfig())
	c.Assert(err, jc.ErrorIsNil)
	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(modelConfig, nil)

	// No new tools available.
	fakeToolFinder := func(_ context.Context, _ tools.SimplestreamsFetcher, _ environs.BootstrapEnviron, _ int, _ int, _ []string, _ coretools.Filter) (coretools.List, error) {
		return nil, errors.NotFoundf("tools")
	}

	// Update should never be called.
	fakeUpdate := func(_ *state.Model, v version.Number) error {
		c.Fail()
		return nil
	}

	api, err := NewAgentToolsAPI(&mockState{}, getDummyEnviron, fakeToolFinder, fakeUpdate, nil, loggertesting.WrapCheckLog(c), s.modelConfigService, s.modelAgentService)
	c.Assert(err, jc.ErrorIsNil)

	err = api.updateToolsAvailability(context.Background())
	c.Assert(err, jc.ErrorIsNil)
}

func getDummyEnviron() (environs.Environ, error) {
	return dummyEnviron{}, nil
}
