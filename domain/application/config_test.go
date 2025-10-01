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
			value: int64(42),
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
			wantError: "expected int64 value, got string",
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
		got, err := encodeApplicationConfigValue(t.value, t.vType)
		if t.wantError != "" {
			c.Assert(err, tc.ErrorMatches, t.wantError)
			c.Check(got, tc.Equals, "")
		} else {
			c.Assert(err, tc.ErrorIsNil)
			c.Check(got, tc.Equals, t.want)
		}
	}
}

func (s *configSuite) TestEncodeApplicationConfig(c *tc.C) {
	tests := []struct {
		name        string
		config      internalcharm.Config
		charmConfig charm.Config
		want        map[string]AddApplicationConfig
		wantErr     string
	}{
		{
			name:   "empty config returns nil",
			config: internalcharm.Config{},
			charmConfig: charm.Config{
				Options: map[string]charm.Option{},
			},
			want: nil,
		},
		{
			name: "single string option",
			config: internalcharm.Config{
				"foo": "bar",
			},
			charmConfig: charm.Config{
				Options: map[string]charm.Option{
					"foo": {Type: charm.OptionString},
				},
			},
			want: map[string]AddApplicationConfig{
				"foo": {Value: "bar", Type: charm.OptionString},
			},
		},
		{
			name: "multiple types",
			config: internalcharm.Config{
				"s": "baz",
				"i": int64(42),
				"f": 3.14,
				"b": true,
			},
			charmConfig: charm.Config{
				Options: map[string]charm.Option{
					"s": {Type: charm.OptionString},
					"i": {Type: charm.OptionInt},
					"f": {Type: charm.OptionFloat},
					"b": {Type: charm.OptionBool},
				},
			},
			want: map[string]AddApplicationConfig{
				"s": {Value: "baz", Type: charm.OptionString},
				"i": {Value: "42", Type: charm.OptionInt},
				"f": {Value: "3.14", Type: charm.OptionFloat},
				"b": {Value: "true", Type: charm.OptionBool},
			},
		},
		{
			name: "missing charm config option",
			config: internalcharm.Config{
				"missing": "value",
			},
			charmConfig: charm.Config{
				Options: map[string]charm.Option{},
			},
			wantErr: `missing charm config, expected "missing"`,
		},
		{
			name: "type mismatch error",
			config: internalcharm.Config{
				"foo": 123,
			},
			charmConfig: charm.Config{
				Options: map[string]charm.Option{
					"foo": {Type: charm.OptionString},
				},
			},
			wantErr: `encoding config value for "foo": expected string value, got int`,
		},
		{
			name: "unsupported option type error",
			config: internalcharm.Config{
				"bar": "baz",
			},
			charmConfig: charm.Config{
				Options: map[string]charm.Option{
					"bar": {Type: charm.OptionType("unknown")},
				},
			},
			wantErr: `encoding config value for "bar": unsupported option type "unknown"`,
		},
	}

	for _, tt := range tests {
		c.Logf("Running test case %q", tt.name)
		got, err := EncodeApplicationConfig(tt.config, tt.charmConfig)
		if tt.wantErr != "" {
			c.Assert(err, tc.ErrorMatches, tt.wantErr)
			c.Check(got, tc.IsNil)
		} else {
			c.Assert(err, tc.ErrorIsNil)
			c.Check(got, tc.DeepEquals, tt.want)
		}
	}
}
