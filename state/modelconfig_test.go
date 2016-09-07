// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"strings"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/mongo/mongotest"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
)

type ModelConfigSuite struct {
	ConnSuite
}

var _ = gc.Suite(&ModelConfigSuite{})

func (s *ModelConfigSuite) SetUpTest(c *gc.C) {
	s.ControllerInheritedConfig = map[string]interface{}{
		"apt-mirror": "http://cloud-mirror",
	}
	s.RegionConfig = cloud.RegionConfig{
		"nether-region": cloud.Attrs{
			"apt-mirror": "http://nether-region-mirror",
			"no-proxy":   "nether-proxy",
		},
		"dummy-region": cloud.Attrs{
			"no-proxy":     "dummy-proxy",
			"image-stream": "dummy-image-stream",
			"whimsy-key":   "whimsy-value",
		},
	}
	s.ConnSuite.SetUpTest(c)
	s.policy.GetConstraintsValidator = func() (constraints.Validator, error) {
		validator := constraints.NewValidator()
		validator.RegisterConflicts([]string{constraints.InstanceType}, []string{constraints.Mem})
		validator.RegisterUnsupported([]string{constraints.CpuPower})
		return validator, nil
	}
	s.policy.GetProviderConfigSchemaSource = func() (config.ConfigSchemaSource, error) {
		return &statetesting.MockConfigSchemaSource{}, nil
	}
}

func (s *ModelConfigSuite) TestAdditionalValidation(c *gc.C) {
	updateAttrs := map[string]interface{}{"logging-config": "juju=ERROR"}
	configValidator1 := func(updateAttrs map[string]interface{}, removeAttrs []string, oldConfig *config.Config) error {
		c.Assert(updateAttrs, jc.DeepEquals, map[string]interface{}{"logging-config": "juju=ERROR"})
		if lc, found := updateAttrs["logging-config"]; found && lc != "" {
			return errors.New("cannot change logging-config")
		}
		return nil
	}
	removeAttrs := []string{"some-attr"}
	configValidator2 := func(updateAttrs map[string]interface{}, removeAttrs []string, oldConfig *config.Config) error {
		c.Assert(removeAttrs, jc.DeepEquals, []string{"some-attr"})
		for _, i := range removeAttrs {
			if i == "some-attr" {
				return errors.New("cannot remove some-attr")
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
	c.Assert(err, gc.ErrorMatches, "cannot remove some-attr")
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

	c.Assert(oldCfg, jc.DeepEquals, cfg)
}

func (s *ModelConfigSuite) TestComposeNewModelConfig(c *gc.C) {
	attrs := map[string]interface{}{
		"authorized-keys": "different-keys",
		"arbitrary-key":   "shazam!",
		"uuid":            testing.ModelTag.Id(),
		"type":            "dummy",
		"name":            "test",
		"resource-tags":   map[string]string{"a": "b", "c": "d"},
	}

	cfgAttrs, err := s.State.ComposeNewModelConfig(
		attrs, &environs.RegionSpec{
			Cloud:  "dummy",
			Region: "dummy-region"})
	c.Assert(err, jc.ErrorIsNil)
	expectedCfg, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	expected := expectedCfg.AllAttrs()
	expected["apt-mirror"] = "http://cloud-mirror"
	expected["providerAttr"] = "vulch"
	expected["whimsy-key"] = "whimsy-value"
	expected["image-stream"] = "dummy-image-stream"
	expected["no-proxy"] = "dummy-proxy"
	// config.New() adds logging-config so remove it.
	expected["logging-config"] = ""
	c.Assert(cfgAttrs, jc.DeepEquals, expected)
}

func (s *ModelConfigSuite) TestComposeNewModelConfigRegionMisses(c *gc.C) {
	attrs := map[string]interface{}{
		"authorized-keys": "different-keys",
		"arbitrary-key":   "shazam!",
		"uuid":            testing.ModelTag.Id(),
		"type":            "dummy",
		"name":            "test",
		"resource-tags":   map[string]string{"a": "b", "c": "d"},
	}
	rspec := &environs.RegionSpec{Cloud: "dummy", Region: "dummy-region"}
	cfgAttrs, err := s.State.ComposeNewModelConfig(attrs, rspec)
	c.Assert(err, jc.ErrorIsNil)
	expectedCfg, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	expected := expectedCfg.AllAttrs()
	expected["apt-mirror"] = "http://cloud-mirror"
	expected["providerAttr"] = "vulch"
	expected["whimsy-key"] = "whimsy-value"
	expected["no-proxy"] = "dummy-proxy"
	expected["image-stream"] = "dummy-image-stream"
	// config.New() adds logging-config so remove it.
	expected["logging-config"] = ""
	c.Assert(cfgAttrs, jc.DeepEquals, expected)
}

func (s *ModelConfigSuite) TestComposeNewModelConfigRegionInherits(c *gc.C) {
	attrs := map[string]interface{}{
		"authorized-keys": "different-keys",
		"arbitrary-key":   "shazam!",
		"uuid":            testing.ModelTag.Id(),
		"type":            "dummy",
		"name":            "test",
		"resource-tags":   map[string]string{"a": "b", "c": "d"},
	}
	rspec := &environs.RegionSpec{Cloud: "dummy", Region: "nether-region"}
	cfgAttrs, err := s.State.ComposeNewModelConfig(attrs, rspec)
	c.Assert(err, jc.ErrorIsNil)
	expectedCfg, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	expected := expectedCfg.AllAttrs()
	expected["no-proxy"] = "nether-proxy"
	expected["apt-mirror"] = "http://nether-region-mirror"
	expected["providerAttr"] = "vulch"
	// config.New() adds logging-config so remove it.
	expected["logging-config"] = ""
	c.Assert(cfgAttrs, jc.DeepEquals, expected)
}

func (s *ModelConfigSuite) TestUpdateModelConfigRejectsControllerConfig(c *gc.C) {
	updateAttrs := map[string]interface{}{"api-port": 1234}
	err := s.State.UpdateModelConfig(updateAttrs, nil, nil)
	c.Assert(err, gc.ErrorMatches, `cannot set controller attribute "api-port" on a model`)
}

func (s *ModelConfigSuite) TestUpdateModelConfigRemoveInherited(c *gc.C) {
	attrs := map[string]interface{}{
		"apt-mirror":    "http://different-mirror", // controller
		"arbitrary-key": "shazam!",
		"providerAttr":  "beef", // provider
		"whimsy-key":    "eggs", // region
	}
	err := s.State.UpdateModelConfig(attrs, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.UpdateModelConfig(nil, []string{"apt-mirror", "arbitrary-key", "providerAttr", "whimsy-key"}, nil)
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	allAttrs := cfg.AllAttrs()
	c.Assert(allAttrs["apt-mirror"], gc.Equals, "http://cloud-mirror")
	c.Assert(allAttrs["providerAttr"], gc.Equals, "vulch")
	c.Assert(allAttrs["whimsy-key"], gc.Equals, "whimsy-value")
	_, ok := allAttrs["arbitrary-key"]
	c.Assert(ok, jc.IsFalse)
}

func (s *ModelConfigSuite) TestUpdateModelConfigCoerce(c *gc.C) {
	attrs := map[string]interface{}{
		"resource-tags": map[string]string{"a": "b", "c": "d"},
	}
	err := s.State.UpdateModelConfig(attrs, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	modelSettings, err := s.State.ReadSettings(state.SettingsC, state.ModelGlobalKey)
	c.Assert(err, jc.ErrorIsNil)
	expectedTags := map[string]string{"a": "b", "c": "d"}
	tagsStr := config.CoerceForStorage(modelSettings.Map())["resource-tags"].(string)
	tagItems := strings.Split(tagsStr, " ")
	tagsMap := make(map[string]string)
	for _, kv := range tagItems {
		parts := strings.Split(kv, "=")
		tagsMap[parts[0]] = parts[1]
	}
	c.Assert(tagsMap, gc.DeepEquals, expectedTags)

	cfg, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.AllAttrs()["resource-tags"], gc.DeepEquals, expectedTags)
}

func (s *ModelConfigSuite) TestUpdateModelConfigPreferredOverRemove(c *gc.C) {
	attrs := map[string]interface{}{
		"apt-mirror":    "http://different-mirror", // controller
		"arbitrary-key": "shazam!",
		"providerAttr":  "beef", // provider
	}
	err := s.State.UpdateModelConfig(attrs, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.UpdateModelConfig(map[string]interface{}{
		"apt-mirror":   "http://another-mirror",
		"providerAttr": "pork",
	}, []string{"apt-mirror", "arbitrary-key"}, nil)
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	allAttrs := cfg.AllAttrs()
	c.Assert(allAttrs["apt-mirror"], gc.Equals, "http://another-mirror")
	c.Assert(allAttrs["providerAttr"], gc.Equals, "pork")
	_, ok := allAttrs["arbitrary-key"]
	c.Assert(ok, jc.IsFalse)
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
	s.RegionConfig = cloud.RegionConfig{
		"dummy-region": cloud.Attrs{
			"apt-mirror": "http://dummy-mirror",
			"no-proxy":   "dummy-proxy",
		},
	}
	s.ConnSuite.SetUpTest(c)

	localControllerSettings, err := s.State.ReadSettings(state.GlobalSettingsC, state.ControllerInheritedSettingsGlobalKey)
	c.Assert(err, jc.ErrorIsNil)
	localControllerSettings.Set("apt-mirror", "http://mirror")
	_, err = localControllerSettings.Write()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ModelConfigSourceSuite) TestModelConfigWhenSetOverridesControllerValue(c *gc.C) {
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
		Config: cfg, Owner: owner, CloudName: "dummy", CloudRegion: "nether-region",
		StorageProviderRegistry: storage.StaticProviderRegistry{},
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

func (s *ModelConfigSourceSuite) assertModelConfigValues(c *gc.C, modelCfg *config.Config, modelAttributes, controllerAttributes set.Strings) {
	expectedValues := make(config.ConfigValues)
	defaultAttributes := set.NewStrings()
	for defaultAttr := range config.ConfigDefaults() {
		defaultAttributes.Add(defaultAttr)
	}
	for attr, val := range modelCfg.AllAttrs() {
		source := "model"
		if defaultAttributes.Contains(attr) {
			source = "default"
		}
		if modelAttributes.Contains(attr) {
			source = "model"
		}
		if controllerAttributes.Contains(attr) {
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

func (s *ModelConfigSourceSuite) TestModelConfigValues(c *gc.C) {
	modelCfg, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	modelAttributes := set.NewStrings("name", "apt-mirror", "logging-config", "authorized-keys", "resource-tags")
	s.assertModelConfigValues(c, modelCfg, modelAttributes, set.NewStrings("http-proxy"))
}

func (s *ModelConfigSourceSuite) TestModelConfigUpdateSource(c *gc.C) {
	attrs := map[string]interface{}{
		"http-proxy": "http://anotherproxy",
		"apt-mirror": "http://mirror",
	}
	err := s.State.UpdateModelConfig(attrs, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	modelCfg, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	modelAttributes := set.NewStrings("name", "http-proxy", "logging-config", "authorized-keys", "resource-tags")
	s.assertModelConfigValues(c, modelCfg, modelAttributes, set.NewStrings("apt-mirror"))
}

func (s *ModelConfigSourceSuite) TestModelConfigDefaults(c *gc.C) {
	expectedValues := make(config.ModelDefaultAttributes)
	for attr, val := range config.ConfigDefaults() {
		expectedValues[attr] = config.AttributeDefaultValues{
			Default: val,
		}
	}
	ds := expectedValues["http-proxy"]
	ds.Controller = "http://proxy"
	expectedValues["http-proxy"] = ds

	ds = expectedValues["apt-mirror"]
	ds.Controller = "http://mirror"
	ds.Regions = []config.RegionDefaultValue{{
		Name:  "dummy-region",
		Value: "http://dummy-mirror",
	}}
	expectedValues["apt-mirror"] = ds

	ds = expectedValues["no-proxy"]
	ds.Regions = []config.RegionDefaultValue{{
		Name:  "dummy-region",
		Value: "dummy-proxy"}}
	expectedValues["no-proxy"] = ds

	sources, err := s.State.ModelConfigDefaultValues()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sources, jc.DeepEquals, expectedValues)
}

func (s *ModelConfigSourceSuite) TestUpdateModelConfigDefaults(c *gc.C) {
	// Set up values that will be removed.
	attrs := map[string]interface{}{
		"http-proxy":  "http://http-proxy",
		"https-proxy": "https://https-proxy",
	}
	err := s.State.UpdateModelConfigDefaultValues(attrs, nil)
	c.Assert(err, jc.ErrorIsNil)

	attrs = map[string]interface{}{
		"apt-mirror": "http://different-mirror",
	}
	err = s.State.UpdateModelConfigDefaultValues(attrs, []string{"http-proxy", "https-proxy"})
	c.Assert(err, jc.ErrorIsNil)

	info := statetesting.NewMongoInfo()
	anotherState, err := state.Open(s.modelTag, s.State.ControllerTag(), info, mongotest.DialOpts(), state.NewPolicyFunc(nil))
	c.Assert(err, jc.ErrorIsNil)
	defer anotherState.Close()

	cfg, err := anotherState.ModelConfigDefaultValues()
	c.Assert(err, jc.ErrorIsNil)
	expectedValues := make(config.ModelDefaultAttributes)
	for attr, val := range config.ConfigDefaults() {
		expectedValues[attr] = config.AttributeDefaultValues{
			Default: val,
		}
	}
	delete(expectedValues, "http-mirror")
	delete(expectedValues, "https-mirror")
	expectedValues["apt-mirror"] = config.AttributeDefaultValues{
		Controller: "http://different-mirror",
		Default:    "",
		Regions: []config.RegionDefaultValue{{
			Name:  "dummy-region",
			Value: "http://dummy-mirror",
		}}}
	expectedValues["no-proxy"] = config.AttributeDefaultValues{
		Default: "",
		Regions: []config.RegionDefaultValue{{
			Name:  "dummy-region",
			Value: "dummy-proxy",
		}}}
	c.Assert(cfg, jc.DeepEquals, expectedValues)
}
