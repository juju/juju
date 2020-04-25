// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"fmt"

	"github.com/juju/charm/v7"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/collections/set"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/cmd/juju/application"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
)

type ApplicationConfigSuite struct {
	jujutesting.JujuConnSuite

	appName string
	charm   *state.Charm
	apiUnit *uniter.Unit

	settingKeys set.Strings
}

func (s *ApplicationConfigSuite) assertApplicationDeployed(c *gc.C) {
	// Create application with all available config field types [currently string, int, boolean, float]
	// where each type has 3 settings:
	// * one with a default;
	// * one with no default;
	// * one will be set to a value at application deploy.
	s.appName = "appconfig"
	s.charm = s.AddTestingCharm(c, s.appName)

	// Deploy application with custom config overwriting desired settings.
	app, err := s.State.AddApplication(state.AddApplicationArgs{
		Name:             s.appName,
		Charm:            s.charm,
		EndpointBindings: nil,
		CharmConfig: map[string]interface{}{
			"stroverwrite":     "test value",
			"intoverwrite":     1620,
			"floatoverwrite":   2.1,
			"booleanoverwrite": false,
			// nil values supplied by the user used to be a problem, bug#1667199
			"booleandefault": nil,
			"floatdefault":   nil,
			"intdefault":     nil,
			"strdefault":     nil,
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit.SetCharmURL(s.charm.URL())
	c.Assert(err, jc.ErrorIsNil)

	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)

	st := s.OpenAPIAs(c, unit.Tag(), password)
	uniter, err := st.Uniter()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uniter, gc.NotNil)

	s.apiUnit, err = uniter.Unit(unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)

	// Ensure both outputs have all charm config keys
	s.settingKeys = set.NewStrings()
	for k := range s.charm.Config().Options {
		s.settingKeys.Add(k)
	}
}

func (s *ApplicationConfigSuite) configCommandOutput(c *gc.C, args ...string) string {
	context, err := cmdtesting.RunCommand(c, application.NewConfigCommand(), args...)
	c.Assert(err, jc.ErrorIsNil)
	return cmdtesting.Stdout(context)
}

func (s *ApplicationConfigSuite) getHookOutput(c *gc.C) charm.Settings {
	settings, err := s.apiUnit.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	return settings
}

// The primary of objective of this test is to ensure
// that both 'juju get' as well as unit in a hook context, uniter.unit, agree
// on all returned settings and values.
// These implementations are separate and cannot be re-factored. However,
// since the logic and expected output is equivalent, these should be modified in sync.
func (s *ApplicationConfigSuite) TestConfigAndConfigGetReturnAllCharmSettings(c *gc.C) {
	// initial deploy with custom settings
	s.assertApplicationDeployed(c)
	s.assertSameConfigOutput(c, initialConfig)

	// use 'juju config foo=' to change values
	s.configCommandOutput(c, s.appName,
		"booleandefault=false",
		"booleannodefault=true",
		"booleanoverwrite=true", //charm default
		"floatdefault=7.2",
		"floatnodefault=10.2",
		"floatoverwrite=11.1", //charm default
		"intdefault=22",
		"intnodefault=11",
		"intoverwrite=111", //charm default
		"strdefault=not",
		"strnodefault=maybe",
		"stroverwrite=me",
	)
	s.assertSameConfigOutput(c, updatedConfig)

	// 'juju config --reset' to reset settings to charm default
	s.configCommandOutput(c, s.appName, "--reset",
		"booleandefault,booleannodefault,booleanoverwrite,floatdefault,"+
			"floatnodefault,floatoverwrite,intdefault,intnodefault,intoverwrite,"+
			"strdefault,strnodefault,stroverwrite")
	s.assertSameConfigOutput(c, resetConfig)
}

func (s *ApplicationConfigSuite) TestConfigNoValueSingleSetting(c *gc.C) {
	appName := "appconfigsingle"
	charm := s.AddTestingCharm(c, appName)
	_, err := s.State.AddApplication(state.AddApplicationArgs{
		Name:  appName,
		Charm: charm,
	})
	c.Assert(err, jc.ErrorIsNil)

	// use 'juju config foo' to see values
	for option := range charm.Config().Options {
		output := s.configCommandOutput(c, appName, option)
		c.Assert(output, gc.Equals, "")
	}
	// set value to be something so that we can check newline added
	s.configCommandOutput(c, appName, "stremptydefault=a")
	output := s.configCommandOutput(c, appName, "stremptydefault")
	c.Assert(output, gc.Equals, "a")
}

func (s *ApplicationConfigSuite) assertSameConfigOutput(c *gc.C, expectedValues settingsMap) {
	s.WaitForModelWatchersIdle(c, s.State.ModelUUID())
	s.assertJujuConfigOutput(c, s.configCommandOutput(c, s.appName), expectedValues)
	s.assertHookOutput(c, s.getHookOutput(c), expectedValues)
}

func (s *ApplicationConfigSuite) assertHookOutput(c *gc.C, obtained charm.Settings, expected settingsMap) {
	c.Assert(len(obtained), gc.Equals, len(expected))
	c.Assert(len(obtained), gc.Equals, len(s.settingKeys))
	for name, aSetting := range expected {
		c.Assert(s.settingKeys.Contains(name), jc.IsTrue)
		// due to awesome float64/int parsing confusion, it's actually safer to ensure that
		// values' string representations match
		c.Assert(fmt.Sprintf("%v", obtained[name]), gc.DeepEquals, fmt.Sprintf("%v", aSetting.Value))
	}
}

func (s *ApplicationConfigSuite) assertJujuConfigOutput(c *gc.C, jujuConfigOutput string, expected settingsMap) {
	var appSettings ApplicationSetting
	err := yaml.Unmarshal([]byte(jujuConfigOutput), &appSettings)
	c.Assert(err, jc.ErrorIsNil)
	obtained := appSettings.Settings

	c.Assert(len(obtained), gc.Equals, len(expected))
	c.Assert(len(obtained), gc.Equals, len(s.settingKeys))
	for name, aSetting := range expected {
		c.Assert(s.settingKeys.Contains(name), jc.IsTrue)
		c.Assert(obtained[name].Value, gc.Equals, aSetting.Value)
		c.Assert(obtained[name].Source, gc.Equals, aSetting.Source)
	}
}

type configSetting struct {
	Value  interface{}
	Source string
}

type settingsMap map[string]configSetting

var (
	initialConfig = settingsMap{
		"booleandefault":   {true, "default"},
		"booleannodefault": {nil, "unset"},
		"booleanoverwrite": {false, "user"},
		"floatdefault":     {4.2, "default"},
		"floatnodefault":   {nil, "unset"},
		"floatoverwrite":   {2.1, "user"},
		"intdefault":       {42, "default"},
		"intnodefault":     {nil, "unset"},
		"intoverwrite":     {1620, "user"},
		"strdefault":       {"charm default", "default"},
		"strnodefault":     {nil, "unset"},
		"stroverwrite":     {"test value", "user"},
	}
	updatedConfig = settingsMap{
		"booleandefault":   {false, "user"},
		"booleannodefault": {true, "user"},
		"booleanoverwrite": {true, "default"},
		"floatdefault":     {7.2, "user"},
		"floatnodefault":   {10.2, "user"},
		"floatoverwrite":   {11.1, "default"},
		"intdefault":       {22, "user"},
		"intnodefault":     {11, "user"},
		"intoverwrite":     {111, "default"},
		"strdefault":       {"not", "user"},
		"strnodefault":     {"maybe", "user"},
		"stroverwrite":     {"me", "user"},
	}
	resetConfig = settingsMap{
		"booleandefault":   {true, "default"},
		"booleannodefault": {nil, "unset"},
		"booleanoverwrite": {true, "default"},
		"floatdefault":     {4.2, "default"},
		"floatnodefault":   {nil, "unset"},
		"floatoverwrite":   {11.1, "default"},
		"intdefault":       {42, "default"},
		"intnodefault":     {nil, "unset"},
		"intoverwrite":     {111, "default"},
		"strdefault":       {"charm default", "default"},
		"strnodefault":     {nil, "unset"},
		"stroverwrite":     {"overwrite me", "default"},
	}
)

type TestSetting struct {
	Default     interface{} `yaml:"default"`
	Description string      `yaml:"description"`
	Source      string      `yaml:"source"`
	Type        string      `yaml:"type"`
	Value       interface{} `yaml:"value"`
}

type ApplicationSetting struct {
	Application string                 `yaml:"application"`
	Charm       string                 `yaml:"charm"`
	Settings    map[string]TestSetting `yaml:"settings"`
}
