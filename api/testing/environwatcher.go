// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/watcher/watchertest"
)

type ModelWatcherFacade interface {
	WatchForModelConfigChanges() (watcher.NotifyWatcher, error)
	ModelConfig() (*config.Config, error)
}

type ModelWatcherTests struct {
	facade ModelWatcherFacade
	state  *state.State
}

func NewModelWatcherTests(
	facade ModelWatcherFacade,
	st *state.State,
) *ModelWatcherTests {
	return &ModelWatcherTests{
		facade: facade,
		state:  st,
	}
}

func (s *ModelWatcherTests) TestModelConfig(c *gc.C) {
	envConfig, err := s.state.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)

	conf, err := s.facade.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(conf, jc.DeepEquals, envConfig)
}

func (s *ModelWatcherTests) TestWatchForModelConfigChanges(c *gc.C) {
	envConfig, err := s.state.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)

	w, err := s.facade.WatchForModelConfigChanges()
	c.Assert(err, jc.ErrorIsNil)
	wc := watchertest.NewNotifyWatcherC(c, w, s.state.StartSync)
	defer wc.AssertStops()

	// Initial event.
	wc.AssertOneChange()

	// Change the model configuration by updating an existing attribute, check it's detected.
	newAttrs := map[string]interface{}{"logging-config": "juju=ERROR"}
	err = s.state.UpdateModelConfig(newAttrs, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Change the model configuration by adding a new attribute, check it's detected.
	newAttrs = map[string]interface{}{"foo": "bar"}
	err = s.state.UpdateModelConfig(newAttrs, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Change the model configuration by removing an attribute, check it's detected.
	err = s.state.UpdateModelConfig(map[string]interface{}{}, []string{"foo"}, nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Change it back to the original config.
	oldAttrs := map[string]interface{}{"logging-config": envConfig.AllAttrs()["logging-config"]}
	err = s.state.UpdateModelConfig(oldAttrs, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
}
