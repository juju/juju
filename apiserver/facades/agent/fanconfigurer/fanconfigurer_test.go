// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package fanconfigurer_test

import (
	"context"
	"fmt"

	"github.com/juju/clock"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/agent/fanconfigurer"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/watcher/registry"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type fanConfigurerSuite struct {
	testing.BaseSuite

	watcherRegistry facade.WatcherRegistry
}

var _ = gc.Suite(&fanConfigurerSuite{})

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

func (s *fanConfigurerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	var err error
	s.watcherRegistry, err = registry.NewRegistry(clock.WallClock)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, s.watcherRegistry) })
}

func (s *fanConfigurerSuite) TearDownTest(c *gc.C) {
	dummy.Reset(c)
	s.BaseSuite.TearDownTest(c)
}

func (s *fanConfigurerSuite) TestWatchSuccess(c *gc.C) {
	authorizer := apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("0"),
	}
	e, err := fanconfigurer.NewFanConfigurerAPIForModel(
		&fakeModelAccessor{},
		s.watcherRegistry,
		authorizer,
	)
	c.Assert(err, jc.ErrorIsNil)
	result, err := e.WatchForFanConfigChanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResult{NotifyWatcherId: "1", Error: nil})
	c.Assert(s.watcherRegistry.Count(), gc.Equals, 1)
}

func (s *fanConfigurerSuite) TestWatchAuthFailed(c *gc.C) {
	authorizer := apiservertesting.FakeAuthorizer{
		Tag: names.NewUserTag("vito"),
	}
	_, err := fanconfigurer.NewFanConfigurerAPIForModel(
		&fakeModelAccessor{},
		s.watcherRegistry,
		authorizer,
	)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *fanConfigurerSuite) TestFanConfigSuccess(c *gc.C) {
	authorizer := apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}
	testingEnvConfig := testingEnvConfig(c)
	e, err := fanconfigurer.NewFanConfigurerAPIForModel(
		&fakeModelAccessor{
			modelConfig: testingEnvConfig,
		},
		s.watcherRegistry,
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

func (s *fanConfigurerSuite) TestFanConfigFetchError(c *gc.C) {
	authorizer := apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}
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
