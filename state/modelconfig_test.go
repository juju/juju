// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
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

func (s *ModelConfigSuite) TestUpdateModelConfigRejectsControllerConfig(c *gc.C) {
	updateAttrs := map[string]interface{}{"api-port": 1234}
	err := s.State.UpdateModelConfig(updateAttrs, nil, nil)
	c.Assert(err, gc.ErrorMatches, `cannot set controller attribute "api-port" on a model`)
}

type ModelConfigSourceSuite struct {
	ConnSuite
}

var _ = gc.Suite(&ModelConfigSourceSuite{})

func (s *ModelConfigSourceSuite) SetUpTest(c *gc.C) {
	s.ControllerInheritedConfig = map[string]interface{}{
		"apt-mirror": "http://cloud-mirror",
		"http-proxy": "http://proxy",
	}
	s.ConnSuite.SetUpTest(c)

	localControllerSettings, err := s.State.ReadSettings(state.GlobalSettingsC, state.ControllerInheritedSettingsGlobalKey)
	c.Assert(err, jc.ErrorIsNil)
	localControllerSettings.Set("apt-mirror", "http://mirror")
	_, err = localControllerSettings.Write()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ModelConfigSourceSuite) TestModelConfigWhenSetOverridesCloudValue(c *gc.C) {
	attrs := map[string]interface{}{
		"authorized-keys": "different-keys",
		"apt-mirror":      "http://anothermirror",
	}
	err := s.State.UpdateModelConfig(attrs, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	cfg, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.AllAttrs()["apt-mirror"], gc.Equals, "http://anothermirror")
}

func (s *ModelConfigSourceSuite) TestControllerModelConfigForksControllerValue(c *gc.C) {
	modelCfg, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelCfg.AllAttrs()["apt-mirror"], gc.Equals, "http://cloud-mirror")

	// Change the local controller settings and ensure the model setting stays the same.
	localControllerSettings, err := s.State.ReadSettings(state.GlobalSettingsC, state.ControllerInheritedSettingsGlobalKey)
	c.Assert(err, jc.ErrorIsNil)
	localControllerSettings.Set("apt-mirror", "http://anothermirror")
	_, err = localControllerSettings.Write()
	c.Assert(err, jc.ErrorIsNil)

	modelCfg, err = s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelCfg.AllAttrs()["apt-mirror"], gc.Equals, "http://cloud-mirror")
}

func (s *ModelConfigSourceSuite) TestNewModelConfigForksControllerValue(c *gc.C) {
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	cfg := testing.CustomModelConfig(c, testing.Attrs{
		"name": "another",
		"uuid": uuid.String(),
	})
	owner := names.NewUserTag("test@remote")
	_, st, err := s.State.NewModel(state.ModelArgs{
		Config: cfg, Owner: owner, CloudName: "dummy",
	})
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	modelCfg, err := st.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelCfg.AllAttrs()["apt-mirror"], gc.Equals, "http://mirror")

	// Change the local controller settings and ensure the model setting stays the same.
	localCloudSettings, err := s.State.ReadSettings(state.GlobalSettingsC, state.ControllerInheritedSettingsGlobalKey)
	c.Assert(err, jc.ErrorIsNil)
	localCloudSettings.Set("apt-mirror", "http://anothermirror")
	_, err = localCloudSettings.Write()
	c.Assert(err, jc.ErrorIsNil)

	modelCfg, err = st.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelCfg.AllAttrs()["apt-mirror"], gc.Equals, "http://mirror")
}

func (s *ModelConfigSourceSuite) TestModelConfigValues(c *gc.C) {
	modelCfg, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	expectedValues := make(config.ConfigValues)
	for attr, val := range modelCfg.AllAttrs() {
		source := "model"
		if attr == "apt-mirror" || attr == "http-proxy" {
			source = "controller"
		}
		expectedValues[attr] = config.ConfigValue{
			Value:  val,
			Source: source,
		}
	}
	sources, err := s.State.ModelConfigValues()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sources, jc.DeepEquals, expectedValues)
}

func (s *ModelConfigSourceSuite) TestModelConfigUpdateSetsSource(c *gc.C) {
	attrs := map[string]interface{}{
		"http-proxy": "http://anotherproxy",
	}
	err := s.State.UpdateModelConfig(attrs, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	modelCfg, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	expectedValues := make(config.ConfigValues)
	for attr, val := range modelCfg.AllAttrs() {
		source := "model"
		if attr == "apt-mirror" {
			source = "controller"
		}
		expectedValues[attr] = config.ConfigValue{
			Value:  val,
			Source: source,
		}
	}
	sources, err := s.State.ModelConfigValues()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sources, jc.DeepEquals, expectedValues)
}

func (s *ModelConfigSourceSuite) TestModelConfigDeleteSetsSource(c *gc.C) {
	err := s.State.UpdateModelConfig(nil, []string{"apt-mirror"}, nil)
	c.Assert(err, jc.ErrorIsNil)

	modelCfg, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	expectedValues := make(config.ConfigValues)
	for attr, val := range modelCfg.AllAttrs() {
		source := "model"
		if attr == "http-proxy" {
			source = "controller"
		}
		expectedValues[attr] = config.ConfigValue{
			Value:  val,
			Source: source,
		}
	}
	sources, err := s.State.ModelConfigValues()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sources, jc.DeepEquals, expectedValues)
}
