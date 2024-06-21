// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/charm"
	internalcharm "github.com/juju/juju/internal/charm"
)

type configSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&configSuite{})

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

func (s *metadataSuite) TestConvertConfig(c *gc.C) {
	for _, tc := range configTestCases {
		c.Logf("Running test case %q", tc.name)

		result, err := decodeConfig(tc.input)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(result, gc.DeepEquals, tc.output)
	}
}
