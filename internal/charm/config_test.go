// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm_test

import (
	"bytes"
	"fmt"
	"strings"
	stdtesting "testing"

	"github.com/juju/tc"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/internal/charm"
)

type ConfigSuite struct {
	config *charm.Config
}

func TestConfigSuite(t *stdtesting.T) { tc.Run(t, &ConfigSuite{}) }
func (s *ConfigSuite) SetUpSuite(c *tc.C) {
	// Just use a single shared config for the whole suite. There's no use case
	// for mutating a config, we assume that nobody will do so here.
	var err error
	s.config, err = charm.ReadConfig(bytes.NewBuffer([]byte(`
options:
  title:
    default: My Title
    description: A descriptive title used for the application.
    type: string
  subtitle:
    default: ""
    description: An optional subtitle used for the application.
  outlook:
    description: No default outlook.
    # type defaults to string in python
  username:
    default: admin001
    description: The name of the initial account (given admin permissions).
    type: string
  skill-level:
    description: A number indicating skill.
    type: int
  agility-ratio:
    description: A number from 0 to 1 indicating agility.
    type: float
  reticulate-splines:
    description: Whether to reticulate splines on launch, or not.
    type: boolean
  secret-foo:
    description: A secret value.
    type: secret
`)))
	c.Assert(err, tc.IsNil)
}

func (s *ConfigSuite) TestReadSample(c *tc.C) {
	c.Assert(s.config.Options, tc.DeepEquals, map[string]charm.Option{
		"title": {
			Default:     "My Title",
			Description: "A descriptive title used for the application.",
			Type:        "string",
		},
		"subtitle": {
			Default:     "",
			Description: "An optional subtitle used for the application.",
			Type:        "string",
		},
		"username": {
			Default:     "admin001",
			Description: "The name of the initial account (given admin permissions).",
			Type:        "string",
		},
		"outlook": {
			Description: "No default outlook.",
			Type:        "string",
		},
		"skill-level": {
			Description: "A number indicating skill.",
			Type:        "int",
		},
		"agility-ratio": {
			Description: "A number from 0 to 1 indicating agility.",
			Type:        "float",
		},
		"reticulate-splines": {
			Description: "Whether to reticulate splines on launch, or not.",
			Type:        "boolean",
		},
		"secret-foo": {
			Description: "A secret value.",
			Type:        "secret",
		},
	})
}

func (s *ConfigSuite) TestDefaultSettings(c *tc.C) {
	c.Assert(s.config.DefaultSettings(), tc.DeepEquals, charm.Settings{
		"title":              "My Title",
		"subtitle":           "",
		"username":           "admin001",
		"secret-foo":         nil,
		"outlook":            nil,
		"skill-level":        nil,
		"agility-ratio":      nil,
		"reticulate-splines": nil,
	})
}

func (s *ConfigSuite) TestFilterSettings(c *tc.C) {
	settings := s.config.FilterSettings(charm.Settings{
		"title":              "something valid",
		"username":           nil,
		"unknown":            "whatever",
		"outlook":            "",
		"skill-level":        5.5,
		"agility-ratio":      true,
		"reticulate-splines": "hullo",
	})
	c.Assert(settings, tc.DeepEquals, charm.Settings{
		"title":    "something valid",
		"username": nil,
		"outlook":  "",
	})
}

func (s *ConfigSuite) TestValidateSettings(c *tc.C) {
	for i, test := range []struct {
		info   string
		input  charm.Settings
		expect charm.Settings
		err    string
	}{
		{
			info:   "nil settings are valid",
			expect: charm.Settings{},
		}, {
			info:  "empty settings are valid",
			input: charm.Settings{},
		}, {
			info:  "unknown keys are not valid",
			input: charm.Settings{"foo": nil},
			err:   `unknown option "foo"`,
		}, {
			info: "nil is valid for every value type",
			input: charm.Settings{
				"outlook":            nil,
				"skill-level":        nil,
				"agility-ratio":      nil,
				"reticulate-splines": nil,
			},
		}, {
			info: "correctly-typed values are valid",
			input: charm.Settings{
				"outlook":            "stormy",
				"skill-level":        int64(123),
				"agility-ratio":      0.5,
				"reticulate-splines": true,
			},
		}, {
			info:   "empty string-typed values stay empty",
			input:  charm.Settings{"outlook": ""},
			expect: charm.Settings{"outlook": ""},
		}, {
			info: "almost-correctly-typed values are valid",
			input: charm.Settings{
				"skill-level":   123,
				"agility-ratio": float32(0.5),
			},
			expect: charm.Settings{
				"skill-level":   int64(123),
				"agility-ratio": 0.5,
			},
		}, {
			info:  "bad string",
			input: charm.Settings{"outlook": false},
			err:   `option "outlook" expected string, got false`,
		}, {
			info:  "bad int",
			input: charm.Settings{"skill-level": 123.4},
			err:   `option "skill-level" expected int, got 123.4`,
		}, {
			info:  "bad float",
			input: charm.Settings{"agility-ratio": "cheese"},
			err:   `option "agility-ratio" expected float, got "cheese"`,
		}, {
			info:  "bad boolean",
			input: charm.Settings{"reticulate-splines": 101},
			err:   `option "reticulate-splines" expected boolean, got 101`,
		}, {
			info:  "invalid secret",
			input: charm.Settings{"secret-foo": "cheese"},
			err:   `option "secret-foo" expected secret, got "cheese"`,
		}, {
			info:   "valid secret",
			input:  charm.Settings{"secret-foo": "secret:cj4v5vm78ohs79o84r4g"},
			expect: charm.Settings{"secret-foo": "secret:cj4v5vm78ohs79o84r4g"},
		},
	} {
		c.Logf("test %d: %s", i, test.info)
		result, err := s.config.ValidateSettings(test.input)
		if test.err != "" {
			c.Check(err, tc.ErrorMatches, test.err)
		} else {
			c.Check(err, tc.IsNil)
			if test.expect == nil {
				c.Check(result, tc.DeepEquals, test.input)
			} else {
				c.Check(result, tc.DeepEquals, test.expect)
			}
		}
	}
}

var settingsWithNils = charm.Settings{
	"outlook":            nil,
	"skill-level":        nil,
	"agility-ratio":      nil,
	"reticulate-splines": nil,
}

var settingsWithValues = charm.Settings{
	"outlook":            "whatever",
	"skill-level":        int64(123),
	"agility-ratio":      2.22,
	"reticulate-splines": true,
}

func (s *ConfigSuite) TestParseSettingsYAML(c *tc.C) {
	for i, test := range []struct {
		info   string
		yaml   string
		key    string
		expect charm.Settings
		err    string
	}{{
		info: "bad structure",
		yaml: "`",
		err:  `cannot parse settings data: .*`,
	}, {
		info: "bad key",
		yaml: "{}",
		key:  "blah",
		err:  `no settings found for "blah"`,
	}, {
		info: "bad settings key",
		yaml: "blah:\n  ping: pong",
		key:  "blah",
		err:  `unknown option "ping"`,
	}, {
		info: "bad type for string",
		yaml: "blah:\n  outlook: 123",
		key:  "blah",
		err:  `option "outlook" expected string, got 123`,
	}, {
		info: "bad type for int",
		yaml: "blah:\n  skill-level: 12.345",
		key:  "blah",
		err:  `option "skill-level" expected int, got 12.345`,
	}, {
		info: "bad type for float",
		yaml: "blah:\n  agility-ratio: blob",
		key:  "blah",
		err:  `option "agility-ratio" expected float, got "blob"`,
	}, {
		info: "bad type for boolean",
		yaml: "blah:\n  reticulate-splines: 123",
		key:  "blah",
		err:  `option "reticulate-splines" expected boolean, got 123`,
	}, {
		info: "bad string for int",
		yaml: "blah:\n  skill-level: cheese",
		key:  "blah",
		err:  `option "skill-level" expected int, got "cheese"`,
	}, {
		info: "bad string for float",
		yaml: "blah:\n  agility-ratio: blob",
		key:  "blah",
		err:  `option "agility-ratio" expected float, got "blob"`,
	}, {
		info: "bad string for boolean",
		yaml: "blah:\n  reticulate-splines: cannonball",
		key:  "blah",
		err:  `option "reticulate-splines" expected boolean, got "cannonball"`,
	}, {
		info:   "empty dict is valid",
		yaml:   "blah: {}",
		key:    "blah",
		expect: charm.Settings{},
	}, {
		info: "nil values are valid",
		yaml: `blah:
            outlook: null
            skill-level: null
            agility-ratio: null
            reticulate-splines: null`,
		key:    "blah",
		expect: settingsWithNils,
	}, {
		info: "empty strings for bool options are not accepted",
		yaml: `blah:
            outlook: ""
            skill-level: 123
            agility-ratio: 12.0
            reticulate-splines: ""`,
		key: "blah",
		err: `option "reticulate-splines" expected boolean, got ""`,
	}, {
		info: "empty strings for int options are not accepted",
		yaml: `blah:
            outlook: ""
            skill-level: ""
            agility-ratio: 12.0
            reticulate-splines: false`,
		key: "blah",
		err: `option "skill-level" expected int, got ""`,
	}, {
		info: "empty strings for float options are not accepted",
		yaml: `blah:
            outlook: ""
            skill-level: 123
            agility-ratio: ""
            reticulate-splines: false`,
		key: "blah",
		err: `option "agility-ratio" expected float, got ""`,
	}, {
		info: "appropriate strings are valid",
		yaml: `blah:
            outlook: whatever
            skill-level: "123"
            agility-ratio: "2.22"
            reticulate-splines: "true"`,
		key:    "blah",
		expect: settingsWithValues,
	}, {
		info: "appropriate types are valid",
		yaml: `blah:
            outlook: whatever
            skill-level: 123
            agility-ratio: 2.22
            reticulate-splines: y`,
		key:    "blah",
		expect: settingsWithValues,
	}} {
		c.Logf("test %d: %s", i, test.info)
		result, err := s.config.ParseSettingsYAML([]byte(test.yaml), test.key)
		if test.err != "" {
			c.Check(err, tc.ErrorMatches, test.err)
		} else {
			c.Check(err, tc.IsNil)
			c.Check(result, tc.DeepEquals, test.expect)
		}
	}
}

func (s *ConfigSuite) TestParseSettingsStrings(c *tc.C) {
	for i, test := range []struct {
		info   string
		input  map[string]string
		expect charm.Settings
		err    string
	}{{
		info:   "nil map is valid",
		expect: charm.Settings{},
	}, {
		info:   "empty map is valid",
		input:  map[string]string{},
		expect: charm.Settings{},
	}, {
		info:   "empty strings for string options are valid",
		input:  map[string]string{"outlook": ""},
		expect: charm.Settings{"outlook": ""},
	}, {
		info:  "empty strings for non-string options are invalid",
		input: map[string]string{"skill-level": ""},
		err:   `option "skill-level" expected int, got ""`,
	}, {
		info: "strings are converted",
		input: map[string]string{
			"outlook":            "whatever",
			"skill-level":        "123",
			"agility-ratio":      "2.22",
			"reticulate-splines": "true",
		},
		expect: settingsWithValues,
	}, {
		info:  "bad string for int",
		input: map[string]string{"skill-level": "cheese"},
		err:   `option "skill-level" expected int, got "cheese"`,
	}, {
		info:  "bad string for float",
		input: map[string]string{"agility-ratio": "blob"},
		err:   `option "agility-ratio" expected float, got "blob"`,
	}, {
		info:  "bad string for boolean",
		input: map[string]string{"reticulate-splines": "cannonball"},
		err:   `option "reticulate-splines" expected boolean, got "cannonball"`,
	}} {
		c.Logf("test %d: %s", i, test.info)
		result, err := s.config.ParseSettingsStrings(test.input)
		if test.err != "" {
			c.Check(err, tc.ErrorMatches, test.err)
		} else {
			c.Check(err, tc.IsNil)
			c.Check(result, tc.DeepEquals, test.expect)
		}
	}
}

func (s *ConfigSuite) TestConfigError(c *tc.C) {
	_, err := charm.ReadConfig(bytes.NewBuffer([]byte(`options: {t: {type: foo}}`)))
	c.Assert(err, tc.ErrorMatches, `invalid config: option "t" has unknown type "foo"`)
}

func (s *ConfigSuite) TestConfigWithNoOptions(c *tc.C) {
	_, err := charm.ReadConfig(strings.NewReader("other:\n"))
	c.Assert(err, tc.ErrorMatches, "invalid config: empty configuration")

	_, err = charm.ReadConfig(strings.NewReader("\n"))
	c.Assert(err, tc.ErrorMatches, "invalid config: empty configuration")

	_, err = charm.ReadConfig(strings.NewReader("null\n"))
	c.Assert(err, tc.ErrorMatches, "invalid config: empty configuration")

	_, err = charm.ReadConfig(strings.NewReader("options:\n"))
	c.Assert(err, tc.IsNil)
}

func (s *ConfigSuite) TestDefaultType(c *tc.C) {
	assertDefault := func(type_ string, value string, expected interface{}) {
		config := fmt.Sprintf(`options: {x: {type: %s, default: %s}}`, type_, value)
		result, err := charm.ReadConfig(bytes.NewBuffer([]byte(config)))
		c.Assert(err, tc.IsNil)
		c.Assert(result.Options["x"].Default, tc.Equals, expected)
	}

	assertDefault("boolean", "true", true)
	assertDefault("string", "golden grahams", "golden grahams")
	assertDefault("string", `""`, "")
	assertDefault("float", "2.211", 2.211)
	assertDefault("int", "99", int64(99))

	assertTypeError := func(type_, str, value string) {
		config := fmt.Sprintf(`options: {t: {type: %s, default: %s}}`, type_, str)
		_, err := charm.ReadConfig(bytes.NewBuffer([]byte(config)))
		expected := fmt.Sprintf(`invalid config default: option "t" expected %s, got %s`, type_, value)
		c.Assert(err, tc.ErrorMatches, expected)
	}

	assertTypeError("boolean", "henry", `"henry"`)
	assertTypeError("string", "2.5", "2.5")
	assertTypeError("float", "123a", `"123a"`)
	assertTypeError("int", "true", "true")
}

// When an empty config is supplied an error should be returned
func (s *ConfigSuite) TestEmptyConfigReturnsError(c *tc.C) {
	config := ""
	result, err := charm.ReadConfig(bytes.NewBuffer([]byte(config)))
	c.Assert(result, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "invalid config: empty configuration")
}

func (s *ConfigSuite) TestYAMLMarshal(c *tc.C) {
	cfg, err := charm.ReadConfig(strings.NewReader(`
options:
    minimal:
        type: string
    withdescription:
        type: int
        description: d
    withdefault:
        type: boolean
        description: d
        default: true
`))
	c.Assert(err, tc.IsNil)
	c.Assert(cfg.Options, tc.HasLen, 3)

	newYAML, err := yaml.Marshal(cfg)
	c.Assert(err, tc.IsNil)

	newCfg, err := charm.ReadConfig(bytes.NewReader(newYAML))
	c.Assert(err, tc.IsNil)
	c.Assert(newCfg, tc.DeepEquals, cfg)
}

func (s *ConfigSuite) TestErrorOnInvalidOptionTypes(c *tc.C) {
	cfg := charm.Config{
		Options: map[string]charm.Option{"testOption": {Type: "invalid type"}},
	}
	_, err := cfg.ParseSettingsYAML([]byte("testKey:\n  testOption: 12.345"), "testKey")
	c.Assert(err, tc.ErrorMatches, "option \"testOption\" has unknown type \"invalid type\"")

	_, err = cfg.ParseSettingsYAML([]byte("testKey:\n  testOption: \"some string value\""), "testKey")
	c.Assert(err, tc.ErrorMatches, "option \"testOption\" has unknown type \"invalid type\"")
}
