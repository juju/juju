// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

type ModelConfigSuite struct {
	ConnSuite
}

var _ = gc.Suite(&ModelConfigSuite{})

func (s *ModelConfigSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.policy.GetConstraintsValidator = func(*config.Config, state.SupportedArchitecturesQuerier) (constraints.Validator, error) {
		validator := constraints.NewValidator()
		validator.RegisterConflicts([]string{constraints.InstanceType}, []string{constraints.Mem})
		validator.RegisterUnsupported([]string{constraints.CpuPower})
		return validator, nil
	}
}

func (s *ModelConfigSuite) TestAdditionalValidation(c *gc.C) {
	updateAttrs := map[string]interface{}{"logging-config": "juju=ERROR"}
	configValidator1 := func(updateAttrs map[string]interface{}, removeAttrs []string, oldConfig *config.Config) error {
		c.Assert(updateAttrs, gc.DeepEquals, map[string]interface{}{"logging-config": "juju=ERROR"})
		if _, found := updateAttrs["logging-config"]; found {
			return fmt.Errorf("cannot change logging-config")
		}
		return nil
	}
	removeAttrs := []string{"logging-config"}
	configValidator2 := func(updateAttrs map[string]interface{}, removeAttrs []string, oldConfig *config.Config) error {
		c.Assert(removeAttrs, gc.DeepEquals, []string{"logging-config"})
		for _, i := range removeAttrs {
			if i == "logging-config" {
				return fmt.Errorf("cannot remove logging-config")
			}
		}
		return nil
	}
	configValidator3 := func(updateAttrs map[string]interface{}, removeAttrs []string, oldConfig *config.Config) error {
		return nil
	}

	err := s.State.UpdateModelConfig(updateAttrs, nil, configValidator1)
	c.Assert(err, gc.ErrorMatches, "cannot change logging-config")
	err = s.State.UpdateModelConfig(nil, removeAttrs, configValidator2)
	c.Assert(err, gc.ErrorMatches, "cannot remove logging-config")
	err = s.State.UpdateModelConfig(updateAttrs, nil, configValidator3)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ModelConfigSuite) TestModelConfig(c *gc.C) {
	attrs := map[string]interface{}{
		"authorized-keys": "different-keys",
		"arbitrary-key":   "shazam!",
	}
	cfg, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.UpdateModelConfig(attrs, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	cfg, err = cfg.Apply(attrs)
	c.Assert(err, jc.ErrorIsNil)
	oldCfg, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(oldCfg, gc.DeepEquals, cfg)
}

func (s *ModelConfigSuite) TestModelConfigIgnoresSameControllerValue(c *gc.C) {
	controllerSettings, err := s.State.ReadSettings(state.ControllersC, "controllerSettings")
	c.Assert(err, jc.ErrorIsNil)
	controllerSettings.Set("apt-mirror", "http://mirror")
	_, err = controllerSettings.Write()
	c.Assert(err, jc.ErrorIsNil)

	attrs := map[string]interface{}{
		"authorized-keys": "different-keys",
		"apt-mirror":      "http://mirror",
	}
	err = s.State.UpdateModelConfig(attrs, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	modelSettings, err := state.ReadSettings(s.State, state.SettingsC, "model")
	c.Assert(err, jc.ErrorIsNil)
	_, ok := modelSettings.Get("apt-mirror")
	c.Assert(ok, jc.IsFalse)
	keys, ok := modelSettings.Get("authorized-keys")
	c.Assert(ok, jc.IsTrue)
	c.Assert(keys, gc.Equals, "different-keys")
}

func (s *ModelConfigSuite) TestModelConfigInheritsControllerValue(c *gc.C) {
	controllerSettings, err := s.State.ReadSettings(state.ControllersC, "controllerSettings")
	c.Assert(err, jc.ErrorIsNil)
	controllerSettings.Set("apt-mirror", "http://mirror")
	_, err = controllerSettings.Write()
	c.Assert(err, jc.ErrorIsNil)

	cfg, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.AllAttrs()["apt-mirror"], gc.Equals, "http://mirror")
}

func (s *ModelConfigSuite) TestModelConfigRetainsSpecialValues(c *gc.C) {
	controllerSettings, err := s.State.ReadSettings(state.ControllersC, "controllerSettings")
	c.Assert(err, jc.ErrorIsNil)

	attrs := make(map[string]interface{})
	for _, attr := range state.RetainModelConfigAttributes {
		v, ok := controllerSettings.Get(attr)
		c.Assert(ok, jc.IsTrue)
		c.Assert(v, gc.Not(gc.Equals), "")
		attrs[attr] = v
	}

	err = s.State.UpdateModelConfig(attrs, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	modelSettings, err := state.ReadSettings(s.State, state.SettingsC, "model")
	c.Assert(err, jc.ErrorIsNil)
	for _, attr := range state.RetainModelConfigAttributes {
		modelV, ok := modelSettings.Get(attr)
		c.Assert(ok, jc.IsTrue)
		controllerV, ok := controllerSettings.Get(attr)
		c.Assert(ok, jc.IsTrue)
		c.Assert(modelV, gc.Equals, controllerV)
	}
}
