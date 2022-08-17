// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

type ModelWatcherFacade interface {
	WatchForModelConfigChanges() (watcher.NotifyWatcher, error)
	ModelConfig() (*config.Config, error)
}

type ModelWatcherTests struct {
	facade ModelWatcherFacade
	state  *state.State
	model  *state.Model
}

func NewModelWatcherTests(
	facade ModelWatcherFacade,
	st *state.State,
	m *state.Model,
) *ModelWatcherTests {
	return &ModelWatcherTests{
		facade: facade,
		state:  st,
		model:  m,
	}
}

func (s *ModelWatcherTests) TestModelConfig(c *gc.C) {
	modelConfig, err := s.model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)

	conf, err := s.facade.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(conf, jc.DeepEquals, modelConfig)
}

func (s *ModelWatcherTests) TestWatchForModelConfigChanges(c *gc.C) {
	modelConfig, err := s.model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)

	w, err := s.facade.WatchForModelConfigChanges()
	c.Assert(err, jc.ErrorIsNil)
	wc := watchertest.NewNotifyWatcherC(c, w)
	defer wc.AssertStops()

	// Initial event.
	wc.AssertOneChange()

	// Change the model configuration by updating an existing attribute, check it's detected.
	newAttrs := map[string]interface{}{"logging-config": "juju=ERROR"}
	err = s.model.UpdateModelConfig(newAttrs, nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Change the model configuration by adding a new attribute, check it's detected.
	newAttrs = map[string]interface{}{"foo": "bar"}
	err = s.model.UpdateModelConfig(newAttrs, nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Change the model configuration by removing an attribute, check it's detected.
	err = s.model.UpdateModelConfig(map[string]interface{}{}, []string{"foo"})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Change it back to the original config.
	oldAttrs := map[string]interface{}{"logging-config": modelConfig.AllAttrs()["logging-config"]}
	err = s.model.UpdateModelConfig(oldAttrs, nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
}
