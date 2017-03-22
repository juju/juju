// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/cmd/juju/application"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type ApplicationConfigSuite struct {
	jujutesting.JujuConnSuite

	appName string
	charm   *state.Charm
	apiUnit *uniter.Unit

	settingKeys set.Strings
}

func (s *ApplicationConfigSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

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
		Settings: map[string]interface{}{
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

	unit, err := app.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	unit.SetCharmURL(s.charm.URL())

	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)

	st := s.OpenAPIAs(c, unit.Tag(), password)
	uniteer, err := st.Uniter()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uniteer, gc.NotNil)

	s.apiUnit, err = uniteer.Unit(unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)

	// Ensure both outputs have all charm config keys
	s.settingKeys = set.NewStrings()
	for k, _ := range s.charm.Config().Options {
		s.settingKeys.Add(k)
	}
}

func (s *ApplicationConfigSuite) configCommandOutput(c *gc.C, args ...string) string {
	context, err := testing.RunCommand(c, application.NewConfigCommand(), args...)
	c.Assert(err, jc.ErrorIsNil)
	return testing.Stdout(context)
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

func (s *ApplicationConfigSuite) assertSameConfigOutput(c *gc.C, expectedValues settingsMap) {
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
		c.Assert(obtained[name].Default, gc.Equals, aSetting.Default)
		c.Assert(obtained[name].Value, gc.Equals, aSetting.Value)
	}
}

type configSetting struct {
	Value   interface{}
	Default bool
}

type settingsMap map[string]configSetting

var (
	initialConfig = settingsMap{
		"booleandefault":   {true, true},
		"booleannodefault": {nil, true},
		"booleanoverwrite": {false, false},
		"floatdefault":     {4.2, true},
		"floatnodefault":   {nil, true},
		"floatoverwrite":   {2.1, false},
		"intdefault":       {42, true},
		"intnodefault":     {nil, true},
		"intoverwrite":     {1620, false},
		"strdefault":       {"charm default", true},
		"strnodefault":     {nil, true},
		"stroverwrite":     {"test value", false},
	}
	updatedConfig = settingsMap{
		"booleandefault":   {false, false},
		"booleannodefault": {true, false},
		"booleanoverwrite": {true, true}, // this should be true since user-specified value is the same as default
		"floatdefault":     {7.2, false},
		"floatnodefault":   {10.2, false},
		"floatoverwrite":   {11.1, true}, // this should be true since user-specified value is the same as default
		"intdefault":       {22, false},
		"intnodefault":     {11, false},
		"intoverwrite":     {111, true}, // this should be true since user-specified value is the same as default
		"strdefault":       {"not", false},
		"strnodefault":     {"maybe", false},
		"stroverwrite":     {"me", false},
	}
	resetConfig = settingsMap{
		"booleandefault":   {true, true},
		"booleannodefault": {nil, true},
		"booleanoverwrite": {true, true},
		"floatdefault":     {4.2, true},
		"floatnodefault":   {nil, true},
		"floatoverwrite":   {11.1, true},
		"intdefault":       {42, true},
		"intnodefault":     {nil, true},
		"intoverwrite":     {111, true},
		"strdefault":       {"charm default", true},
		"strnodefault":     {nil, true},
		"stroverwrite":     {"overwrite me", true},
	}
)

type TestSetting struct {
	Default     bool        `yaml:"default"`
	Description string      `yaml:"description"`
	Type        string      `yaml:"type"`
	Value       interface{} `yaml:"value"`
}

type ApplicationSetting struct {
	Application string                 `yaml:"application"`
	Charm       string                 `yaml:"charm"`
	Settings    map[string]TestSetting `yaml:"settings"`
}
