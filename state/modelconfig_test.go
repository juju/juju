// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/constraints"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
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
	s.policy.GetProviderConfigSchemaSource = func(cloudName string) (config.ConfigSchemaSource, error) {
		return &statetesting.MockConfigSchemaSource{CloudName: cloudName}, nil
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

	err := s.Model.UpdateModelConfig(updateAttrs, nil, configValidator1)
	c.Assert(err, gc.ErrorMatches, "cannot change logging-config")
	err = s.Model.UpdateModelConfig(nil, removeAttrs, configValidator2)
	c.Assert(err, gc.ErrorMatches, "cannot remove some-attr")
	err = s.Model.UpdateModelConfig(updateAttrs, nil, configValidator3)
	c.Assert(err, jc.ErrorIsNil)
	// First error is returned.
	err = s.Model.UpdateModelConfig(updateAttrs, nil, configValidator1, configValidator2)
	c.Assert(err, gc.ErrorMatches, "cannot change logging-config")
}

func (s *ModelConfigSuite) TestModelConfig(c *gc.C) {
	attrs := map[string]interface{}{
		"authorized-keys": "different-keys",
		"arbitrary-key":   "shazam!",
	}
	cfg, err := s.Model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	err = s.Model.UpdateModelConfig(attrs, nil)
	c.Assert(err, jc.ErrorIsNil)
	cfg, err = cfg.Apply(attrs)
	c.Assert(err, jc.ErrorIsNil)
	oldCfg, err := s.Model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(oldCfg, jc.DeepEquals, cfg)
}

func (s *ModelConfigSuite) TestAgentVersion(c *gc.C) {
	attrs := map[string]interface{}{
		"agent-version": "2.2.3",
		"arbitrary-key": "shazam!",
	}
	ver, err := s.Model.AgentVersion()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ver, gc.DeepEquals, version.Number{Major: 2, Minor: 0, Patch: 0})

	err = s.Model.UpdateModelConfig(attrs, nil)
	c.Assert(err, jc.ErrorIsNil)

	ver, err = s.Model.AgentVersion()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ver, gc.DeepEquals, version.Number{Major: 2, Minor: 2, Patch: 3})
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
		attrs, &environscloudspec.CloudRegionSpec{
			Cloud:  "dummy",
			Region: "dummy-region"})
	c.Assert(err, jc.ErrorIsNil)
	expectedCfg, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	expected := expectedCfg.AllAttrs()
	expected["apt-mirror"] = "http://cloud-mirror"
	expected["providerAttrdummy"] = "vulch"
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
	rspec := &environscloudspec.CloudRegionSpec{Cloud: "dummy", Region: "dummy-region"}
	cfgAttrs, err := s.State.ComposeNewModelConfig(attrs, rspec)
	c.Assert(err, jc.ErrorIsNil)
	expectedCfg, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	expected := expectedCfg.AllAttrs()
	expected["apt-mirror"] = "http://cloud-mirror"
	expected["providerAttrdummy"] = "vulch"
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
	rspec := &environscloudspec.CloudRegionSpec{Cloud: "dummy", Region: "nether-region"}
	cfgAttrs, err := s.State.ComposeNewModelConfig(attrs, rspec)
	c.Assert(err, jc.ErrorIsNil)
	expectedCfg, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	expected := expectedCfg.AllAttrs()
	expected["no-proxy"] = "nether-proxy"
	expected["apt-mirror"] = "http://nether-region-mirror"
	expected["providerAttrdummy"] = "vulch"
	// config.New() adds logging-config so remove it.
	expected["logging-config"] = ""
	c.Assert(cfgAttrs, jc.DeepEquals, expected)
}

func (s *ModelConfigSuite) TestUpdateModelConfigRejectsControllerConfig(c *gc.C) {
	updateAttrs := map[string]interface{}{"api-port": 1234}
	err := s.Model.UpdateModelConfig(updateAttrs, nil)
	c.Assert(err, gc.ErrorMatches, `cannot set controller attribute "api-port" on a model`)
}

func (s *ModelConfigSuite) TestUpdateModelConfigRemoveInherited(c *gc.C) {
	attrs := map[string]interface{}{
		"apt-mirror":        "http://different-mirror", // controller
		"arbitrary-key":     "shazam!",
		"providerAttrdummy": "beef", // provider
		"whimsy-key":        "eggs", // region
	}
	err := s.Model.UpdateModelConfig(attrs, nil)
	c.Assert(err, jc.ErrorIsNil)

	err = s.Model.UpdateModelConfig(nil, []string{"apt-mirror", "arbitrary-key", "providerAttrdummy", "whimsy-key"})
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := s.Model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	allAttrs := cfg.AllAttrs()
	c.Assert(allAttrs["apt-mirror"], gc.Equals, "http://cloud-mirror")
	c.Assert(allAttrs["providerAttrdummy"], gc.Equals, "vulch")
	c.Assert(allAttrs["whimsy-key"], gc.Equals, "whimsy-value")
	_, ok := allAttrs["arbitrary-key"]
	c.Assert(ok, jc.IsFalse)
}

func (s *ModelConfigSuite) TestUpdateModelConfigCoerce(c *gc.C) {
	attrs := map[string]interface{}{
		"resource-tags": map[string]string{"a": "b", "c": "d"},
	}
	err := s.Model.UpdateModelConfig(attrs, nil)
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

	cfg, err := s.Model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.AllAttrs()["resource-tags"], gc.DeepEquals, expectedTags)
}

func (s *ModelConfigSuite) TestUpdateModelConfigPreferredOverRemove(c *gc.C) {
	attrs := map[string]interface{}{
		"apt-mirror":        "http://different-mirror", // controller
		"arbitrary-key":     "shazam!",
		"providerAttrdummy": "beef", // provider
	}
	err := s.Model.UpdateModelConfig(attrs, nil)
	c.Assert(err, jc.ErrorIsNil)

	err = s.Model.UpdateModelConfig(map[string]interface{}{
		"apt-mirror":        "http://another-mirror",
		"providerAttrdummy": "pork",
	}, []string{"apt-mirror", "arbitrary-key"})
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := s.Model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	allAttrs := cfg.AllAttrs()
	c.Assert(allAttrs["apt-mirror"], gc.Equals, "http://another-mirror")
	c.Assert(allAttrs["providerAttrdummy"], gc.Equals, "pork")
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

	localControllerSettings, err := s.State.ReadSettings(state.GlobalSettingsC, state.CloudGlobalKey("dummy"))
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
	err := s.Model.UpdateModelConfig(attrs, nil)
	c.Assert(err, jc.ErrorIsNil)

	cfg, err := s.Model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.AllAttrs()["apt-mirror"], gc.Equals, "http://anothermirror")
}

func (s *ModelConfigSourceSuite) TestControllerModelConfigForksControllerValue(c *gc.C) {
	modelCfg, err := s.Model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelCfg.AllAttrs()["apt-mirror"], gc.Equals, "http://cloud-mirror")

	// Change the local controller settings and ensure the model setting stays the same.
	localControllerSettings, err := s.State.ReadSettings(state.GlobalSettingsC, state.CloudGlobalKey("dummy"))
	c.Assert(err, jc.ErrorIsNil)
	localControllerSettings.Set("apt-mirror", "http://anothermirror")
	_, err = localControllerSettings.Write()
	c.Assert(err, jc.ErrorIsNil)

	modelCfg, err = s.Model.ModelConfig()
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
	_, st, err := s.Controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		Config:                  cfg,
		Owner:                   owner,
		CloudName:               "dummy",
		CloudRegion:             "nether-region",
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	modelCfg, err := m.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelCfg.AllAttrs()["apt-mirror"], gc.Equals, "http://mirror")

	// Change the local controller settings and ensure the model setting stays the same.
	localCloudSettings, err := s.State.ReadSettings(state.GlobalSettingsC, state.CloudGlobalKey("dummy"))
	c.Assert(err, jc.ErrorIsNil)
	localCloudSettings.Set("apt-mirror", "http://anothermirror")
	_, err = localCloudSettings.Write()
	c.Assert(err, jc.ErrorIsNil)

	modelCfg, err = m.ModelConfig()
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
	sources, err := s.Model.ModelConfigValues()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sources, jc.DeepEquals, expectedValues)
}

func (s *ModelConfigSourceSuite) TestModelConfigValues(c *gc.C) {
	modelCfg, err := s.Model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	modelAttributes := set.NewStrings("name", "apt-mirror", "logging-config", "authorized-keys", "resource-tags")
	s.assertModelConfigValues(c, modelCfg, modelAttributes, set.NewStrings("http-proxy"))
}

func (s *ModelConfigSourceSuite) TestModelConfigUpdateSource(c *gc.C) {
	attrs := map[string]interface{}{
		"http-proxy": "http://anotherproxy",
		"apt-mirror": "http://mirror",
	}
	err := s.Model.UpdateModelConfig(attrs, nil)
	c.Assert(err, jc.ErrorIsNil)
	modelCfg, err := s.Model.ModelConfig()
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

	sources, err := s.State.ModelConfigDefaultValues(s.Model.CloudName())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sources, jc.DeepEquals, expectedValues)
}

func (s *ModelConfigSourceSuite) TestUpdateModelConfigDefaults(c *gc.C) {
	// Set up values that will be removed.
	attrs := map[string]interface{}{
		"http-proxy":  "http://http-proxy",
		"https-proxy": "https://https-proxy",
	}
	err := s.State.UpdateModelConfigDefaultValues(attrs, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	attrs = map[string]interface{}{
		"apt-mirror":            "http://different-mirror",
		"num-provision-workers": 66,
	}
	err = s.State.UpdateModelConfigDefaultValues(attrs, []string{"http-proxy", "https-proxy"}, nil)
	c.Assert(err, jc.ErrorIsNil)

	cfg, err := s.State.ModelConfigDefaultValues(s.Model.CloudName())
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
		Default: "127.0.0.1,localhost,::1",
		Regions: []config.RegionDefaultValue{{
			Name:  "dummy-region",
			Value: "dummy-proxy",
		}}}
	expectedValues["num-provision-workers"] = config.AttributeDefaultValues{
		Controller: 66,
		Default:    16,
	}
	c.Assert(cfg, jc.DeepEquals, expectedValues)
}

func (s *ModelConfigSourceSuite) TestUpdateModelConfigDefaultsArbitraryConfig(c *gc.C) {
	attrs := map[string]interface{}{
		"hello": "world",
	}
	err := s.State.UpdateModelConfigDefaultValues(attrs, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	cfg, err := s.State.ModelConfigDefaultValues(s.Model.CloudName())
	c.Assert(err, jc.ErrorIsNil)
	expectedValues := make(config.ModelDefaultAttributes)
	for attr, val := range config.ConfigDefaults() {
		expectedValues[attr] = config.AttributeDefaultValues{
			Default: val,
		}
	}

	expectedValues["hello"] = config.AttributeDefaultValues{
		Controller: "world",
		Default:    nil,
	}
	expectedValues["http-proxy"] = config.AttributeDefaultValues{
		Controller: "http://proxy",
		Default:    "",
	}
	expectedValues["apt-mirror"] = config.AttributeDefaultValues{
		Controller: "http://mirror",
		Default:    "",
		Regions: []config.RegionDefaultValue{{
			Name:  "dummy-region",
			Value: "http://dummy-mirror",
		}}}
	expectedValues["no-proxy"] = config.AttributeDefaultValues{
		Default: "127.0.0.1,localhost,::1",
		Regions: []config.RegionDefaultValue{{
			Name:  "dummy-region",
			Value: "dummy-proxy",
		}}}
	c.Assert(cfg, jc.DeepEquals, expectedValues)
}

func (s *ModelConfigSourceSuite) TestUpdateModelConfigRegionDefaults(c *gc.C) {
	// The test env is setup with dummy/dummy-region having a no-proxy
	// dummy-proxy value and nether-region with a nether-proxy value.
	//
	// First we change the no-proxy setting in dummy-region
	attrs := map[string]interface{}{
		"no-proxy": "changed-proxy",
	}

	rspec, err := environscloudspec.NewCloudRegionSpec("dummy", "dummy-region")
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.UpdateModelConfigDefaultValues(attrs, nil, rspec)
	c.Assert(err, jc.ErrorIsNil)

	cfg, err := s.State.ModelConfigDefaultValues(s.Model.CloudName())
	c.Assert(err, jc.ErrorIsNil)
	expectedValues := make(config.ModelDefaultAttributes)
	for attr, val := range config.ConfigDefaults() {
		expectedValues[attr] = config.AttributeDefaultValues{
			Default: val,
		}
	}
	expectedValues["http-proxy"] = config.AttributeDefaultValues{
		Controller: "http://proxy",
		Default:    "",
	}
	expectedValues["apt-mirror"] = config.AttributeDefaultValues{
		Controller: "http://mirror",
		Default:    "",
		Regions: []config.RegionDefaultValue{{
			Name:  "dummy-region",
			Value: "http://dummy-mirror",
		}}}
	expectedValues["no-proxy"] = config.AttributeDefaultValues{
		Default: "127.0.0.1,localhost,::1",
		Regions: []config.RegionDefaultValue{{
			Name:  "dummy-region",
			Value: "changed-proxy",
		}}}
	c.Assert(cfg, jc.DeepEquals, expectedValues)

	// remove the dummy-region setting
	err = s.State.UpdateModelConfigDefaultValues(nil, []string{"no-proxy"}, rspec)
	c.Assert(err, jc.ErrorIsNil)

	// and check again
	cfg, err = s.State.ModelConfigDefaultValues(s.Model.CloudName())
	c.Assert(err, jc.ErrorIsNil)
	expectedValues = make(config.ModelDefaultAttributes)
	for attr, val := range config.ConfigDefaults() {
		expectedValues[attr] = config.AttributeDefaultValues{
			Default: val,
		}
	}
	expectedValues["http-proxy"] = config.AttributeDefaultValues{
		Controller: "http://proxy",
		Default:    "",
	}
	expectedValues["apt-mirror"] = config.AttributeDefaultValues{
		Controller: "http://mirror",
		Default:    "",
		Regions: []config.RegionDefaultValue{{
			Name:  "dummy-region",
			Value: "http://dummy-mirror",
		}}}
	c.Assert(cfg, jc.DeepEquals, expectedValues)
}

func (s *ModelConfigSourceSuite) TestUpdateModelConfigDefaultValuesUnknownRegion(c *gc.C) {
	// Set up settings to create
	attrs := map[string]interface{}{
		"no-proxy": "changed-proxy",
	}

	rspec, err := environscloudspec.NewCloudRegionSpec("dummy", "unused-region")
	c.Assert(err, jc.ErrorIsNil)

	// We add this to the unused-region which has not been created in mongo
	// yet.
	err = s.State.UpdateModelConfigDefaultValues(attrs, nil, rspec)
	c.Assert(err, jc.ErrorIsNil)

	// Then check config.
	cfg, err := s.State.ModelConfigDefaultValues(s.Model.CloudName())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg["no-proxy"], jc.DeepEquals, config.AttributeDefaultValues{
		Default:    "127.0.0.1,localhost,::1",
		Controller: nil,
		Regions: []config.RegionDefaultValue{
			{
				Name:  "dummy-region",
				Value: "dummy-proxy",
			}, {
				Name:  "unused-region",
				Value: "changed-proxy",
			}}})
}
