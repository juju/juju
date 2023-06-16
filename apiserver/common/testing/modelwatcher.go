// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type ModelWatcher interface {
	WatchForModelConfigChanges() (params.NotifyWatchResult, error)
	ModelConfig() (params.ModelConfigResult, error)
}

type ModelWatcherTest struct {
	modelWatcher    ModelWatcher
	st              *state.State
	watcherRegistry facade.WatcherRegistry
}

func NewModelWatcherTest(
	modelWatcher ModelWatcher,
	st *state.State,
	watcherRegistry facade.WatcherRegistry,
) *ModelWatcherTest {
	return &ModelWatcherTest{
		modelWatcher:    modelWatcher,
		st:              st,
		watcherRegistry: watcherRegistry,
	}
}

// AssertModelConfig provides a method to test the config from the
// modelWatcher.  This allows other tests that embed this type to have
// more than just the default test.
func (s *ModelWatcherTest) AssertModelConfig(c *gc.C, modelWatcher ModelWatcher) {
	model, err := s.st.Model()
	c.Assert(err, jc.ErrorIsNil)

	modelConfig, err := model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)

	result, err := modelWatcher.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)

	configAttributes := modelConfig.AllAttrs()
	c.Assert(result.Config, jc.DeepEquals, params.ModelConfig(configAttributes))
}

func (s *ModelWatcherTest) TestModelConfig(c *gc.C) {
	s.AssertModelConfig(c, s.modelWatcher)
}

func (s *ModelWatcherTest) TestWatchForModelConfigChanges(c *gc.C) {
	c.Assert(s.watcherRegistry.Count(), gc.Equals, 0)

	result, err := s.modelWatcher.WatchForModelConfigChanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResult{
		NotifyWatcherId: "1",
	})

	// Verify the watcher were registered and stop them when done.
	c.Assert(s.watcherRegistry.Count(), gc.Equals, 1)
	watcher, err := s.watcherRegistry.Get("1")
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, watcher)

	// Check that the Watch has consumed the initial event ("returned"
	// in the Watch call)
	wc := statetesting.NewNotifyWatcherC(c, watcher.(state.NotifyWatcher))
	wc.AssertNoChange()

	workertest.CleanKill(c, watcher)
}
