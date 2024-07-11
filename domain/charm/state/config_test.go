// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/charm"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type configSuite struct {
	schematesting.ModelSuite
}

var _ = gc.Suite(&configSuite{})

var configTestCases = [...]struct {
	name   string
	input  []charmConfig
	output charm.Config
}{
	{
		name:  "empty",
		input: []charmConfig{},
		output: charm.Config{
			Options: make(map[string]charm.Option),
		},
	},
	{
		name: "string",
		input: []charmConfig{
			{
				Key:          "string",
				Type:         "string",
				Description:  "description",
				DefaultValue: "default",
			},
		},
		output: charm.Config{
			Options: map[string]charm.Option{
				"string": {
					Type:        charm.OptionString,
					Description: "description",
					Default:     "default",
				},
			},
		},
	},
	{
		name: "secret",
		input: []charmConfig{
			{
				Key:          "secret",
				Type:         "secret",
				Description:  "description",
				DefaultValue: "default",
			},
		},
		output: charm.Config{
			Options: map[string]charm.Option{
				"secret": {
					Type:        charm.OptionSecret,
					Description: "description",
					Default:     "default",
				},
			},
		},
	},
	{
		name: "int",
		input: []charmConfig{
			{
				Key:          "int",
				Type:         "int",
				Description:  "description",
				DefaultValue: "1",
			},
		},
		output: charm.Config{
			Options: map[string]charm.Option{
				"int": {
					Type:        charm.OptionInt,
					Description: "description",
					Default:     1,
				},
			},
		},
	},
	{
		name: "float",
		input: []charmConfig{
			{
				Key:          "float",
				Type:         "float",
				Description:  "description",
				DefaultValue: "4.2",
			},
		},
		output: charm.Config{
			Options: map[string]charm.Option{
				"float": {
					Type:        charm.OptionFloat,
					Description: "description",
					Default:     4.2,
				},
			},
		},
	},
	{
		name: "boolean",
		input: []charmConfig{
			{
				Key:          "boolean",
				Type:         "boolean",
				Description:  "description",
				DefaultValue: "true",
			},
		},
		output: charm.Config{
			Options: map[string]charm.Option{
				"boolean": {
					Type:        charm.OptionBool,
					Description: "description",
					Default:     true,
				},
			},
		},
	},
}

func (s *configSuite) TestConvertConfig(c *gc.C) {
	for _, tc := range configTestCases {
		c.Logf("Running test case %q", tc.name)

		result, err := decodeConfig(tc.input)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(result, gc.DeepEquals, tc.output)
	}
}
