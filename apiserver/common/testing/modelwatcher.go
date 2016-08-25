// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type ModelWatcher interface {
	WatchForModelConfigChanges() (params.NotifyWatchResult, error)
	ModelConfig() (params.ModelConfigResult, error)
}

type ModelWatcherTest struct {
	modelWatcher ModelWatcher
	st           *state.State
	resources    *common.Resources
}

func NewModelWatcherTest(
	modelWatcher ModelWatcher,
	st *state.State,
	resources *common.Resources,
) *ModelWatcherTest {
	return &ModelWatcherTest{modelWatcher, st, resources}
}

// AssertModelConfig provides a method to test the config from the
// envWatcher.  This allows other tests that embed this type to have
// more than just the default test.
func (s *ModelWatcherTest) AssertModelConfig(c *gc.C, envWatcher ModelWatcher) {
	envConfig, err := s.st.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)

	result, err := envWatcher.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)

	configAttributes := envConfig.AllAttrs()
	c.Assert(result.Config, jc.DeepEquals, params.ModelConfig(configAttributes))
}

func (s *ModelWatcherTest) TestModelConfig(c *gc.C) {
	s.AssertModelConfig(c, s.modelWatcher)
}

func (s *ModelWatcherTest) TestWatchForModelConfigChanges(c *gc.C) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	result, err := s.modelWatcher.WatchForModelConfigChanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResult{
		NotifyWatcherId: "1",
	})

	// Verify the resources were registered and stop them when done.
	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	// Check that the Watch has consumed the initial event ("returned"
	// in the Watch call)
	wc := statetesting.NewNotifyWatcherC(c, s.st, resource.(state.NotifyWatcher))
	wc.AssertNoChange()
}
