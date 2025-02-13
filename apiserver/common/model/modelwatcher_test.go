// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"context"
	"fmt"
	"sync"
	"time"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common/model"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/cmd"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	_ "github.com/juju/juju/internal/provider/dummy"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
)

type modelWatcherSuite struct {
	watcherRegistry    *facademocks.MockWatcherRegistry
	modelConfigService *MockModelConfigService
}

var _ = gc.Suite(&modelWatcherSuite{})

func (s *modelWatcherSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.watcherRegistry = facademocks.NewMockWatcherRegistry(ctrl)
	s.modelConfigService = NewMockModelConfigService(ctrl)
	return ctrl
}

func (s *modelWatcherSuite) TestWatchSuccess(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	done := make(chan struct{})
	defer close(done)
	wg := sync.WaitGroup{}
	defer wg.Wait()
	ch := make(chan []string)
	w := watchertest.NewMockStringsWatcher(ch)

	s.modelConfigService.EXPECT().Watch().DoAndReturn(func() (watcher.Watcher[[]string], error) {
		wg.Add(1)
		time.AfterFunc(testing.ShortWait, func() {
			defer wg.Done()
			// Send initial event.
			select {
			case ch <- []string{}:
			case <-done:
				c.ExpectFailure("watcher did not fire")
			}
		})
		return w, nil
	})
	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("1", nil)

	facade := model.NewModelConfigWatcher(s.modelConfigService, s.watcherRegistry)
	result, err := facade.WatchForModelConfigChanges(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResult{NotifyWatcherId: "1", Error: nil})
}

func (s *modelWatcherSuite) TestWatchFailure(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	wg := sync.WaitGroup{}
	defer wg.Wait()
	ch := make(chan []string)
	w := watchertest.NewMockStringsWatcher(ch)

	s.modelConfigService.EXPECT().Watch().DoAndReturn(func() (watcher.Watcher[[]string], error) {
		wg.Add(1)
		time.AfterFunc(testing.ShortWait, func() {
			defer wg.Done()
			w.KillErr(fmt.Errorf("bad watcher"))
			close(ch)
		})
		return w, nil
	})

	facade := model.NewModelConfigWatcher(s.modelConfigService, s.watcherRegistry)
	result, err := facade.WatchForModelConfigChanges(context.Background())
	c.Assert(err, gc.ErrorMatches, "bad watcher")
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResult{})
}

func (s *modelWatcherSuite) TestModelConfigSuccess(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	testingModelConfig := testingEnvConfig(c)
	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(testingModelConfig, nil)

	facade := model.NewModelConfigWatcher(s.modelConfigService, s.watcherRegistry)
	result, err := facade.ModelConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	// Make sure we can read the secret attribute (i.e. it's not masked).
	c.Check(result.Config["secret"], gc.Equals, "pork")
	c.Check(map[string]any(result.Config), jc.DeepEquals, testingModelConfig.AllAttrs())
}

func (s *modelWatcherSuite) TestModelConfigFetchError(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(nil, fmt.Errorf("nope"))

	facade := model.NewModelConfigWatcher(s.modelConfigService, s.watcherRegistry)
	result, err := facade.ModelConfig(context.Background())
	c.Assert(err, gc.ErrorMatches, "nope")
	c.Check(result.Config, gc.IsNil)
}

func testingEnvConfig(c *gc.C) *config.Config {
	env, err := bootstrap.PrepareController(
		false,
		cmd.BootstrapContext(context.Background(), cmdtesting.Context(c)),
		jujuclient.NewMemStore(),
		bootstrap.PrepareParams{
			ControllerConfig: testing.FakeControllerConfig(),
			ControllerName:   "dummycontroller",
			ModelConfig:      testing.FakeConfig(),
			Cloud:            testing.FakeCloudSpec(),
			AdminSecret:      "admin-secret",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	return env.Config()
}
