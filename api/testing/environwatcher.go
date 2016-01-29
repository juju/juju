// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/watcher/watchertest"
)

const (
	HasSecrets = true
	NoSecrets  = false
)

type ModelWatcherFacade interface {
	WatchForModelConfigChanges() (watcher.NotifyWatcher, error)
	ModelConfig() (*config.Config, error)
}

type ModelWatcherTests struct {
	facade     ModelWatcherFacade
	state      *state.State
	hasSecrets bool
}

func NewModelWatcherTests(
	facade ModelWatcherFacade,
	st *state.State,
	hasSecrets bool) *ModelWatcherTests {
	return &ModelWatcherTests{
		facade:     facade,
		state:      st,
		hasSecrets: hasSecrets,
	}
}

func (s *ModelWatcherTests) TestModelConfig(c *gc.C) {
	envConfig, err := s.state.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)

	conf, err := s.facade.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)

	// If the facade doesn't have secrets, we need to replace the config
	// values in our model to compare against with the secrets replaced.
	if !s.hasSecrets {
		env, err := environs.New(envConfig)
		c.Assert(err, jc.ErrorIsNil)
		secretAttrs, err := env.Provider().SecretAttrs(envConfig)
		c.Assert(err, jc.ErrorIsNil)
		secrets := make(map[string]interface{})
		for key := range secretAttrs {
			secrets[key] = "not available"
		}
		envConfig, err = envConfig.Apply(secrets)
		c.Assert(err, jc.ErrorIsNil)
	}

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
