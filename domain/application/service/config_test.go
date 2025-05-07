// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/tc"
	"github.com/juju/testing"

	"github.com/juju/juju/domain/application/charm"
	internalcharm "github.com/juju/juju/internal/charm"
)

type configSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&configSuite{})

var configTestCases = [...]struct {
	name   string
	input  charm.Config
	output internalcharm.Config
}{
	{
		name:   "empty",
		input:  charm.Config{},
		output: internalcharm.Config{},
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
		output: internalcharm.Config{
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

func (s *metadataSuite) TestConvertConfig(c *tc.C) {
	for _, tc := range configTestCases {
		c.Logf("Running test case %q", tc.name)

		result, err := decodeConfig(tc.input)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(result, tc.DeepEquals, tc.output)

		// Ensure that the conversion is idempotent.
		converted, err := encodeConfig(&result)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(converted, tc.DeepEquals, tc.input)
	}
}
