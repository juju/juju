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

func (s *configSuite) TestEncodeConfigValue(c *tc.C) {
	tests := []struct {
		name      string
		value     any
		vType     charm.OptionType
		want      string
		wantError string
	}{
		{
			name:  "string value",
			value: "foo",
			vType: charm.OptionString,
			want:  "foo",
		},
		{
			name:  "secret value",
			value: "bar",
			vType: charm.OptionSecret,
			want:  "bar",
		},
		{
			name:  "int value",
			value: 42,
			vType: charm.OptionInt,
			want:  "42",
		},
		{
			name:  "float value",
			value: 3.14,
			vType: charm.OptionFloat,
			want:  "3.14",
		},
		{
			name:  "bool true value",
			value: true,
			vType: charm.OptionBool,
			want:  "true",
		},
		{
			name:  "bool false value",
			value: false,
			vType: charm.OptionBool,
			want:  "false",
		},
		{
			name:      "wrong type for string",
			value:     123,
			vType:     charm.OptionString,
			wantError: "expected string value, got int",
		},
		{
			name:      "wrong type for int",
			value:     "notint",
			vType:     charm.OptionInt,
			wantError: "expected int value, got string",
		},
		{
			name:      "wrong type for float",
			value:     "notfloat",
			vType:     charm.OptionFloat,
			wantError: "expected float64 value, got string",
		},
		{
			name:      "wrong type for bool",
			value:     "notbool",
			vType:     charm.OptionBool,
			wantError: "expected bool value, got string",
		},
		{
			name:      "unsupported option type",
			value:     "foo",
			vType:     charm.OptionType("unknown"),
			wantError: `unsupported option type "unknown"`,
		},
	}
	for _, t := range tests {
		c.Logf("Running test case %q", t.name)
		got, err := EncodeApplicationConfigValue(t.value, t.vType)
		if t.wantError != "" {
			c.Assert(err, tc.ErrorMatches, t.wantError)
			c.Check(got, tc.Equals, "")
		} else {
			c.Assert(err, tc.ErrorIsNil)
			c.Check(got, tc.Equals, t.want)
		}
	}
}
