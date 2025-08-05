// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package fanconfigurer_test

import (
	"context"

	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/agent/fanconfigurer"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type fanconfigurerSuite struct {
	testing.BaseSuite

	mockModelAccessor   *MockModelAccessor
	mockMachineAccessor *MockMachineAccessor
	mockMachine         *MockMachine
}

var _ = gc.Suite(&fanconfigurerSuite{})

func (s *fanconfigurerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockModelAccessor = NewMockModelAccessor(ctrl)
	s.mockMachineAccessor = NewMockMachineAccessor(ctrl)
	s.mockMachine = NewMockMachine(ctrl)
	s.AddCleanup(func(_ *gc.C) {
		s.mockModelAccessor = nil
		s.mockMachineAccessor = nil
		s.mockMachine = nil
	})
	return ctrl
}

func (s *fanconfigurerSuite) TestWatchSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	resources := common.NewResources()
	authorizer := apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("0"),
	}
	s.AddCleanup(func(_ *gc.C) { resources.StopAll() })
	s.mockModelAccessor.EXPECT().WatchForModelConfigChanges().Return(apiservertesting.NewFakeNotifyWatcher())

	e, err := fanconfigurer.NewFanConfigurerAPIForModel(
		s.mockModelAccessor,
		s.mockMachineAccessor,
		resources,
		authorizer,
	)
	c.Assert(err, jc.ErrorIsNil)
	result, err := e.WatchForFanConfigChanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResult{"1", nil})
	c.Assert(resources.Count(), gc.Equals, 1)
}

func (s *fanconfigurerSuite) TestWatchAuthFailed(c *gc.C) {
	defer s.setupMocks(c).Finish()

	resources := common.NewResources()
	authorizer := apiservertesting.FakeAuthorizer{
		Tag: names.NewUserTag("vito"),
	}
	s.AddCleanup(func(_ *gc.C) { resources.StopAll() })
	_, err := fanconfigurer.NewFanConfigurerAPIForModel(
		s.mockModelAccessor,
		s.mockMachineAccessor,
		resources,
		authorizer,
	)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *fanconfigurerSuite) TestFanConfigSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	resources := common.NewResources()
	authorizer := apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}
	s.AddCleanup(func(_ *gc.C) { resources.StopAll() })
	testingEnvConfig := testingEnvConfig(c)
	s.mockModelAccessor.EXPECT().ModelConfig().Return(testingEnvConfig, nil)
	s.mockMachineAccessor.EXPECT().Machine("0").Return(s.mockMachine, nil)
	s.mockMachine.EXPECT().Base().Return(state.Base{OS: "ubuntu", Channel: "22.04/stable"})

	e, err := fanconfigurer.NewFanConfigurerAPIForModel(
		s.mockModelAccessor,
		s.mockMachineAccessor,
		resources,
		authorizer,
	)
	c.Assert(err, jc.ErrorIsNil)
	result, err := e.FanConfig(params.Entity{Tag: "machine-0"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Fans, gc.HasLen, 2)
	c.Check(result.Fans[0].Underlay, gc.Equals, "10.100.0.0/16")
	c.Check(result.Fans[0].Overlay, gc.Equals, "251.0.0.0/8")
	c.Check(result.Fans[1].Underlay, gc.Equals, "192.168.0.0/16")
	c.Check(result.Fans[1].Overlay, gc.Equals, "252.0.0.0/8")
}

func (s *fanconfigurerSuite) TestFanConfigNoble(c *gc.C) {
	defer s.setupMocks(c).Finish()

	resources := common.NewResources()
	authorizer := apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}
	s.AddCleanup(func(_ *gc.C) { resources.StopAll() })
	s.mockMachineAccessor.EXPECT().Machine("0").Return(s.mockMachine, nil)
	s.mockMachine.EXPECT().Base().Return(state.Base{OS: "ubuntu", Channel: "24.04/stable"})

	e, err := fanconfigurer.NewFanConfigurerAPIForModel(
		s.mockModelAccessor,
		s.mockMachineAccessor,
		resources,
		authorizer,
	)
	c.Assert(err, jc.ErrorIsNil)
	result, err := e.FanConfig(params.Entity{Tag: "machine-0"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Fans, gc.HasLen, 0)
}

func (s *fanconfigurerSuite) TestFanConfigFetchError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	resources := common.NewResources()
	authorizer := apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}
	s.AddCleanup(func(_ *gc.C) { resources.StopAll() })
	s.mockMachineAccessor.EXPECT().Machine("0").Return(s.mockMachine, nil)
	s.mockMachine.EXPECT().Base().Return(state.Base{OS: "ubuntu", Channel: "22.04/stable"})
	s.mockModelAccessor.EXPECT().ModelConfig().Return(nil, errors.New("pow"))

	e, err := fanconfigurer.NewFanConfigurerAPIForModel(
		s.mockModelAccessor,
		s.mockMachineAccessor,
		nil,
		authorizer,
	)
	c.Assert(err, jc.ErrorIsNil)
	_, err = e.FanConfig(params.Entity{Tag: "machine-0"})
	c.Assert(err, gc.ErrorMatches, "pow")
}

func testingEnvConfig(c *gc.C) *config.Config {
	env, err := bootstrap.PrepareController(
		false,
		modelcmd.BootstrapContext(context.Background(), cmdtesting.Context(c)),
		jujuclient.NewMemStore(),
		bootstrap.PrepareParams{
			ControllerConfig: testing.FakeControllerConfig(),
			ControllerName:   "dummycontroller",
			ModelConfig:      dummy.SampleConfig().Merge(testing.Attrs{"fan-config": "10.100.0.0/16=251.0.0.0/8 192.168.0.0/16=252.0.0.0/8"}),
			Cloud:            dummy.SampleCloudSpec(),
			AdminSecret:      "admin-secret",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	return env.Config()
}
