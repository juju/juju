// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package toolsversionchecker

import (
	context "context"

	"github.com/juju/errors"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/tools"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	coretools "github.com/juju/juju/internal/tools"
)

type ToolsCheckerSuite struct {
	coretesting.BaseSuite

	mockBootstrapEnviron   *MockBootstrapEnviron
	mockModelConfigService *MockModelConfigService
	mockModelAgentService  *MockModelAgentService
	mockMachineService     *MockMachineService
}

func (s *ToolsCheckerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockBootstrapEnviron = NewMockBootstrapEnviron(ctrl)
	s.mockModelConfigService = NewMockModelConfigService(ctrl)
	s.mockModelAgentService = NewMockModelAgentService(ctrl)
	s.mockMachineService = NewMockMachineService(ctrl)

	s.mockMachineService.EXPECT().GetBootstrapEnviron(gomock.Any()).Return(s.mockBootstrapEnviron, nil)

	c.Cleanup(func() {
		s.mockBootstrapEnviron = nil
		s.mockModelConfigService = nil
		s.mockModelAgentService = nil
		s.mockMachineService = nil
	})

	return ctrl
}

func (s *ToolsCheckerSuite) TestCheckTools(c *tc.C) {
	defer s.setupMocks(c).Finish()

	expVer, err := semversion.Parse("2.5.0")
	c.Assert(err, tc.ErrorIsNil)
	s.mockModelAgentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(expVer, nil)
	modelConfig, err := config.New(config.NoDefaults, coretesting.FakeConfig())
	c.Assert(err, tc.ErrorIsNil)
	s.mockModelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(modelConfig, nil)

	var (
		calledWithMajor, calledWithMinor int
	)
	fakeToolFinder := func(_ context.Context, _ tools.SimplestreamsFetcher, _ environs.BootstrapEnviron, maj int, min int, streams []string, filter coretools.Filter) (coretools.List, error) {
		calledWithMajor = maj
		calledWithMinor = min
		ver := semversion.Binary{Number: semversion.Number{Major: maj, Minor: min}}
		t := coretools.Tools{Version: ver, URL: "http://example.com", Size: 1}
		c.Assert(calledWithMajor, tc.Equals, 2)
		c.Assert(calledWithMinor, tc.Equals, 5)
		c.Assert(streams, tc.DeepEquals, []string{"released"})
		return coretools.List{&t}, nil
	}

	w := toolsVersionWorker{
		logger: loggertesting.WrapCheckLog(c),
		domainServices: domainServices{
			config:  s.mockModelConfigService,
			agent:   s.mockModelAgentService,
			machine: s.mockMachineService,
		},
		findTools: fakeToolFinder,
	}

	obtainedVer, err := w.checkToolsAvailability(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtainedVer, tc.Equals, expVer)
}

func (s *ToolsCheckerSuite) TestCheckToolsNonReleasedStream(c *tc.C) {
	defer s.setupMocks(c).Finish()

	expVer, err := semversion.Parse("2.5-alpha1")
	c.Assert(err, tc.ErrorIsNil)
	s.mockModelAgentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(expVer, nil)

	sConfig := coretesting.FakeConfig()
	sConfig = sConfig.Merge(coretesting.Attrs{
		"agent-stream": "proposed",
	})
	cfg, err := config.New(config.NoDefaults, sConfig)
	c.Assert(err, tc.ErrorIsNil)
	s.mockModelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(cfg, nil)

	var (
		calledWithMajor, calledWithMinor int
		calledWithStreams                [][]string
	)
	fakeToolFinder := func(_ context.Context, _ tools.SimplestreamsFetcher, _ environs.BootstrapEnviron, maj int, min int, streams []string, filter coretools.Filter) (coretools.List, error) {
		calledWithMajor = maj
		calledWithMinor = min
		calledWithStreams = append(calledWithStreams, streams)
		if len(streams) == 1 && streams[0] == "released" {
			return nil, coretools.ErrNoMatches
		}
		ver := semversion.Binary{Number: semversion.Number{Major: maj, Minor: min}}
		t := coretools.Tools{Version: ver, URL: "http://example.com", Size: 1}
		c.Assert(calledWithMajor, tc.Equals, 2)
		c.Assert(calledWithMinor, tc.Equals, 5)
		return coretools.List{&t}, nil
	}

	w := toolsVersionWorker{
		logger: loggertesting.WrapCheckLog(c),
		domainServices: domainServices{
			config:  s.mockModelConfigService,
			agent:   s.mockModelAgentService,
			machine: s.mockMachineService,
		},
		findTools: fakeToolFinder,
	}

	obtainedVer, err := w.checkToolsAvailability(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(calledWithStreams, tc.DeepEquals, [][]string{{"proposed", "released"}})
	c.Assert(obtainedVer, tc.Equals, semversion.Number{Major: 2, Minor: 5, Patch: 0})
}

func (s *ToolsCheckerSuite) TestUpdateToolsAvailability(c *tc.C) {
	defer s.setupMocks(c).Finish()

	expVer, err := semversion.Parse("2.5.0")
	c.Assert(err, tc.ErrorIsNil)
	s.mockModelAgentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(expVer, nil)
	modelConfig, err := config.New(config.NoDefaults, coretesting.FakeConfig())
	c.Assert(err, tc.ErrorIsNil)
	s.mockModelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(modelConfig, nil)

	s.mockModelAgentService.EXPECT().UpdateLatestAgentVersion(gomock.Any(), semversion.Number{Major: 2, Minor: 5, Patch: 2}).Return(nil)

	fakeToolFinder := func(_ context.Context, _ tools.SimplestreamsFetcher, _ environs.BootstrapEnviron, _ int, _ int, _ []string, _ coretools.Filter) (coretools.List, error) {
		ver := semversion.Binary{Number: semversion.Number{Major: 2, Minor: 5, Patch: 2}}
		olderVer := semversion.Binary{Number: semversion.Number{Major: 2, Minor: 5, Patch: 1}}
		t := coretools.Tools{Version: ver, URL: "http://example.com", Size: 1}
		tOld := coretools.Tools{Version: olderVer, URL: "http://example.com", Size: 1}
		return coretools.List{&t, &tOld}, nil
	}

	w := toolsVersionWorker{
		logger: loggertesting.WrapCheckLog(c),
		domainServices: domainServices{
			config:  s.mockModelConfigService,
			agent:   s.mockModelAgentService,
			machine: s.mockMachineService,
		},
		findTools: fakeToolFinder,
	}

	err = w.updateToolsAvailability(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ToolsCheckerSuite) TestUpdateToolsAvailabilityNoMatches(c *tc.C) {
	defer s.setupMocks(c).Finish()

	expVer, err := semversion.Parse("2.5.0")
	c.Assert(err, tc.ErrorIsNil)
	s.mockModelAgentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(expVer, nil)
	modelConfig, err := config.New(config.NoDefaults, coretesting.FakeConfig())
	c.Assert(err, tc.ErrorIsNil)
	s.mockModelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(modelConfig, nil)

	// No new tools available.
	fakeToolFinder := func(_ context.Context, _ tools.SimplestreamsFetcher, _ environs.BootstrapEnviron, _ int, _ int, _ []string, _ coretools.Filter) (coretools.List, error) {
		return nil, errors.NotFoundf("tools")
	}

	w := toolsVersionWorker{
		logger: loggertesting.WrapCheckLog(c),
		domainServices: domainServices{
			config:  s.mockModelConfigService,
			agent:   s.mockModelAgentService,
			machine: s.mockMachineService,
		},
		findTools: fakeToolFinder,
	}

	err = w.updateToolsAvailability(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}
