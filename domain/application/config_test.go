// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/domain/application/charm"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/testhelpers"
)

type configSuite struct {
	testhelpers.IsolationSuite
}

func TestConfigSuite(t *testing.T) {
	tc.Run(t, &configSuite{})
}

var configTestCases = [...]struct {
	name   string
	input  charm.Config
	output internalcharm.ConfigSpec
}{
	{
		name:   "empty",
		input:  charm.Config{},
		output: internalcharm.ConfigSpec{},
	},
	{
		name: "config",
		input: charm.Config{
			Options: map[string]charm.Option{
				"key-string": {
					Type:        charm.OptionString,
					Description: "description-string",
					Default:     "default-string",
				},
				"key-int": {
					Type:        charm.OptionInt,
					Description: "description-int",
					Default:     "default-int",
				},
				"key-float": {
					Type:        charm.OptionFloat,
					Description: "description-float",
					Default:     "default-float",
				},
				"key-bool": {
					Type:        charm.OptionBool,
					Description: "description-bool",
					Default:     "default-bool",
				},
				"key-secret": {
					Type:        charm.OptionSecret,
					Description: "description-secret",
					Default:     "default-secret",
				},
			},
		},
		output: internalcharm.ConfigSpec{
			Options: map[string]internalcharm.Option{
				"key-string": {
					Type:        "string",
					Description: "description-string",
					Default:     "default-string",
				},
				"key-int": {
					Type:        "int",
					Description: "description-int",
					Default:     "default-int",
				},
				"key-float": {
					Type:        "float",
					Description: "description-float",
					Default:     "default-float",
				},
				"key-bool": {
					Type:        "boolean",
					Description: "description-bool",
					Default:     "default-bool",
				},
				"key-secret": {
					Type:        "secret",
					Description: "description-secret",
					Default:     "default-secret",
				},
			},
		},
	},
}

func (s *configSuite) TestConvertConfig(c *tc.C) {
	for _, testCase := range configTestCases {
		c.Logf("Running test case %q", testCase.name)

		result, err := DecodeConfig(testCase.input)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(result, tc.DeepEquals, testCase.output)

		// Ensure that the conversion is idempotent.
		converted, err := EncodeConfig(&result)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(converted, tc.DeepEquals, testCase.input)
	}
}

var appConfigTestCase = []struct {
	name     string
	input    map[string]ApplicationConfig
	expected internalcharm.Config
	errMatch string
}{
	{
		name:     "empty config",
		input:    map[string]ApplicationConfig{},
		expected: internalcharm.Config{},
	},
	{
		name: "string value",
		input: map[string]ApplicationConfig{
			"foo": {
				Type:  charm.OptionString,
				Value: ptr("bar"),
			},
		},
		expected: internalcharm.Config{
			"foo": "bar",
		},
	},
	{
		name: "int value",
		input: map[string]ApplicationConfig{
			"num": {
				Type:  charm.OptionInt,
				Value: ptr("42"),
			},
		},
		expected: internalcharm.Config{
			"num": 42,
		},
	},
	{
		name: "float value",
		input: map[string]ApplicationConfig{
			"flt": {
				Type:  charm.OptionFloat,
				Value: ptr("3.14"),
			},
		},
		expected: internalcharm.Config{
			"flt": 3.14,
		},
	},
	{
		name: "bool value true",
		input: map[string]ApplicationConfig{
			"flag": {
				Type:  charm.OptionBool,
				Value: ptr("true"),
			},
		},
		expected: internalcharm.Config{
			"flag": true,
		},
	},
	{
		name: "bool value false",
		input: map[string]ApplicationConfig{
			"flag": {
				Type:  charm.OptionBool,
				Value: ptr("false"),
			},
		},
		expected: internalcharm.Config{
			"flag": false,
		},
	},
	{
		name: "secret value",
		input: map[string]ApplicationConfig{
			"secret": {
				Type:  charm.OptionSecret,
				Value: ptr("s3cr3t"),
			},
		},
		expected: internalcharm.Config{
			"secret": "s3cr3t",
		},
	},
	{
		name: "nil value",
		input: map[string]ApplicationConfig{
			"nilkey": {
				Type:  charm.OptionString,
				Value: nil,
			},
		},
		expected: internalcharm.Config{
			"nilkey": nil,
		},
	},
	{
		name: "invalid int value",
		input: map[string]ApplicationConfig{
			"badint": {
				Type:  charm.OptionInt,
				Value: ptr("notanint"),
			},
		},
		errMatch: ".*cannot convert string \"notanint\" to int.*",
	},
	{
		name: "invalid float value",
		input: map[string]ApplicationConfig{
			"badfloat": {
				Type:  charm.OptionFloat,
				Value: ptr("notafloat"),
			},
		},
		errMatch: ".*cannot convert string \"notafloat\" to float.*",
	},
	{
		name: "invalid bool value",
		input: map[string]ApplicationConfig{
			"badbool": {
				Type:  charm.OptionBool,
				Value: ptr("notabool"),
			},
		},
		errMatch: ".*cannot convert string \"notabool\" to bool.*",
	},
	{
		name: "unknown type",
		input: map[string]ApplicationConfig{
			"unknown": {
				Type:  charm.OptionType("unknown"),
				Value: ptr("value"),
			},
		},
		errMatch: ".*unknown config type \"unknown\".*",
	},
}

func (s *configSuite) TestDecodeApplicationConfig(c *tc.C) {
	for _, t := range appConfigTestCase {
		c.Logf("Running test case %q", t.name)
		result, err := DecodeApplicationConfig(t.input)
		if t.errMatch != "" {
			c.Assert(err, tc.ErrorMatches, t.errMatch)
			c.Check(result, tc.IsNil)
		} else {
			c.Assert(err, tc.ErrorIsNil)
			c.Check(result, tc.DeepEquals, t.expected)
		}
	}
}

// ptr is a helper to get a pointer to a string literal.
func ptr(s string) *string {
	return &s
}
