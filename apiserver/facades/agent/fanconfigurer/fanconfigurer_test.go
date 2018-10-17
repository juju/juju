// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package fanconfigurer_test

import (
	"fmt"

	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/agent/fanconfigurer"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type fanconfigurerSuite struct {
	testing.BaseSuite
	testingEnvConfig *config.Config
}

var _ = gc.Suite(&fanconfigurerSuite{})

type fakeModelAccessor struct {
	modelConfig      *config.Config
	modelConfigError error
}

func (*fakeModelAccessor) WatchForModelConfigChanges() state.NotifyWatcher {
	return apiservertesting.NewFakeNotifyWatcher()
}

func (f *fakeModelAccessor) ModelConfig() (*config.Config, error) {
	if f.modelConfigError != nil {
		return nil, f.modelConfigError
	}
	return f.modelConfig, nil
}

func (s *fanconfigurerSuite) TearDownTest(c *gc.C) {
	dummy.Reset(c)
	s.BaseSuite.TearDownTest(c)
}

func (s *fanconfigurerSuite) TestWatchSuccess(c *gc.C) {
	resources := common.NewResources()
	authorizer := apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("0"),
	}
	s.AddCleanup(func(_ *gc.C) { resources.StopAll() })
	e, err := fanconfigurer.NewFanConfigurerAPIForModel(
		&fakeModelAccessor{},
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
	resources := common.NewResources()
	authorizer := apiservertesting.FakeAuthorizer{
		Tag: names.NewUserTag("vito"),
	}
	s.AddCleanup(func(_ *gc.C) { resources.StopAll() })
	_, err := fanconfigurer.NewFanConfigurerAPIForModel(
		&fakeModelAccessor{},
		resources,
		authorizer,
	)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *fanconfigurerSuite) TestFanConfigSuccess(c *gc.C) {
	resources := common.NewResources()
	authorizer := apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}
	s.AddCleanup(func(_ *gc.C) { resources.StopAll() })
	testingEnvConfig := testingEnvConfig(c)
	e, err := fanconfigurer.NewFanConfigurerAPIForModel(
		&fakeModelAccessor{
			modelConfig: testingEnvConfig,
		},
		resources,
		authorizer,
	)
	c.Assert(err, jc.ErrorIsNil)
	result, err := e.FanConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Fans, gc.HasLen, 2)
	c.Check(result.Fans[0].Underlay, gc.Equals, "10.100.0.0/16")
	c.Check(result.Fans[0].Overlay, gc.Equals, "251.0.0.0/8")
	c.Check(result.Fans[1].Underlay, gc.Equals, "192.168.0.0/16")
	c.Check(result.Fans[1].Overlay, gc.Equals, "252.0.0.0/8")
}

func (s *fanconfigurerSuite) TestFanConfigFetchError(c *gc.C) {
	resources := common.NewResources()
	authorizer := apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}
	s.AddCleanup(func(_ *gc.C) { resources.StopAll() })
	e, err := fanconfigurer.NewFanConfigurerAPIForModel(
		&fakeModelAccessor{
			modelConfigError: fmt.Errorf("pow"),
		},
		nil,
		authorizer,
	)
	c.Assert(err, jc.ErrorIsNil)
	_, err = e.FanConfig()
	c.Assert(err, gc.ErrorMatches, "pow")
}

func testingEnvConfig(c *gc.C) *config.Config {
	env, err := bootstrap.PrepareController(
		false,
		modelcmd.BootstrapContext(cmdtesting.Context(c)),
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
