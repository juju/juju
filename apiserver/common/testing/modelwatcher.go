// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type ModelWatcher interface {
	WatchForModelConfigChanges(context.Context) (params.NotifyWatchResult, error)
	ModelConfig(context.Context) (params.ModelConfigResult, error)
}

type ModelWatcherTest struct {
	modelWatcher ModelWatcher
	st           *state.State
	// We can't call this "resources" as it conflicts
	// when embedded in other test suites.
	res *common.Resources
}

func NewModelWatcherTest(
	modelWatcher ModelWatcher,
	st *state.State,
	resources *common.Resources,
) *ModelWatcherTest {
	return &ModelWatcherTest{modelWatcher: modelWatcher, st: st, res: resources}
}

// AssertModelConfig provides a method to test the config from the
// modelWatcher.  This allows other tests that embed this type to have
// more than just the default test.
func (s *ModelWatcherTest) AssertModelConfig(c *gc.C, modelWatcher ModelWatcher) {
	model, err := s.st.Model()
	c.Assert(err, jc.ErrorIsNil)

	modelConfig, err := model.ModelConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	result, err := modelWatcher.ModelConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	configAttributes := modelConfig.AllAttrs()
	c.Assert(result.Config, jc.DeepEquals, params.ModelConfig(configAttributes))
}

func (s *ModelWatcherTest) TestModelConfig(c *gc.C) {
	s.AssertModelConfig(c, s.modelWatcher)
}

func (s *ModelWatcherTest) TestWatchForModelConfigChanges(c *gc.C) {
	c.Assert(s.res.Count(), gc.Equals, 0)

	result, err := s.modelWatcher.WatchForModelConfigChanges(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResult{
		NotifyWatcherId: "1",
	})

	// Verify the resources were registered and stop them when done.
	c.Assert(s.res.Count(), gc.Equals, 1)
	resource := s.res.Get("1")
	defer workertest.CleanKill(c, resource)

	// Check that the Watch has consumed the initial event ("returned"
	// in the Watch call)
	wc := statetesting.NewNotifyWatcherC(c, resource.(state.NotifyWatcher))
	wc.AssertNoChange()
}
