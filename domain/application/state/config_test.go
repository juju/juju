// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	stdtesting "testing"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	"github.com/juju/juju/domain/application/charm"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type configSuite struct {
	schematesting.ModelSuite
}

func TestConfigSuite(t *stdtesting.T) { tc.Run(t, &configSuite{}) }

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
				DefaultValue: ptr("default"),
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
				DefaultValue: ptr("default"),
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
				DefaultValue: ptr("1"),
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
				DefaultValue: ptr("4.2"),
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
				DefaultValue: ptr("true"),
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
	{
		name: "nil",
		input: []charmConfig{
			{
				Key:         "string",
				Type:        "string",
				Description: "description",
			},
		},
		output: charm.Config{
			Options: map[string]charm.Option{
				"string": {
					Type:        charm.OptionString,
					Description: "description",
				},
			},
		},
	},
}

func (s *configSuite) TestDecodeConfig(c *tc.C) {
	for _, testCase := range configTestCases {
		c.Logf("Running test case %q", testCase.name)

		result, err := decodeConfig(testCase.input)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(result, tc.DeepEquals, testCase.output)
	}
}

func (s *configSuite) TestDecodeConfigType(c *tc.C) {
	_, err := decodeConfigType("invalid")
	c.Assert(err, tc.ErrorMatches, `unknown config type "invalid"`)
}

func (s *configSuite) TestEncodeConfigType(c *tc.C) {
	_, err := decodeConfigType("invalid")
	c.Assert(err, tc.ErrorMatches, `unknown config type "invalid"`)
}

func (s *configSuite) TestEncodeConfigDefaultValue(c *tc.C) {
	_, err := encodeConfigDefaultValue(int32(0))
	c.Assert(err, tc.ErrorMatches, `unknown config default value type int32`)
}

var configTypeTestCases = [...]struct {
	name   string
	kind   charm.OptionType
	input  *string
	output any
}{
	{
		name:   "string",
		kind:   charm.OptionString,
		input:  ptr("deadbeef"),
		output: "deadbeef",
	},
	{
		name:   "int",
		kind:   charm.OptionInt,
		input:  ptr("42"),
		output: 42,
	},
	{
		name:   "float",
		kind:   charm.OptionFloat,
		input:  ptr("42.3"),
		output: 42.3,
	},
	{
		name:   "bool",
		kind:   charm.OptionBool,
		input:  ptr("true"),
		output: true,
	},
	{
		name:   "secret",
		kind:   charm.OptionSecret,
		input:  ptr("ssh"),
		output: "ssh",
	},
	{
		name:   "nil",
		input:  nil,
		output: nil,
	},
}

func (s *configSuite) TestDecodeThenEncodeDefaultValue(c *tc.C) {
	for _, testCase := range configTypeTestCases {
		c.Logf("Running test case %q", testCase.name)

		decoded, err := decodeConfigDefaultValue(testCase.kind, testCase.input)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(decoded, tc.DeepEquals, testCase.output)

		encoded, err := encodeConfigDefaultValue(decoded)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(encoded, tc.DeepEquals, testCase.input)
	}
}

var encodeConfigTypeTestCases = [...]struct {
	name   string
	input  any
	output *string
}{
	{
		name:   "string",
		input:  "deadbeef",
		output: ptr("deadbeef"),
	},
	{
		name:   "int",
		input:  int(42),
		output: ptr("42"),
	},
	{
		name:   "int64",
		input:  int64(42),
		output: ptr("42"),
	},
	{
		name:   "float64",
		input:  float64(42.1),
		output: ptr("42.1"),
	},
	{
		name:   "float64",
		input:  float64(42.0),
		output: ptr("42"),
	},
	{
		name:   "float64",
		input:  float64(42),
		output: ptr("42"),
	},
	{
		name:   "bool",
		input:  true,
		output: ptr("true"),
	},
	{
		name:   "nil",
		input:  nil,
		output: nil,
	},
}

func (s *configSuite) TestEncodeDefaultValue(c *tc.C) {
	for _, testCase := range encodeConfigTypeTestCases {
		c.Logf("Running test case %q", testCase.name)

		encoded, err := encodeConfigDefaultValue(testCase.input)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(encoded, tc.DeepEquals, testCase.output)
	}
}

func (s *configSuite) TestDecodeConfigTypeError(c *tc.C) {
	_, err := decodeConfigDefaultValue(charm.OptionType("invalid"), ptr(""))
	c.Assert(err, tc.Not(tc.ErrorIsNil))
}

type configStateSuite struct {
	schematesting.ModelSuite
}

func TestConfigStateSuite(t *stdtesting.T) { tc.Run(t, &configStateSuite{}) }
func (s *configStateSuite) TestConfigType(c *tc.C) {
	type charmConfigType struct {
		ID   int    `db:"id"`
		Name string `db:"name"`
	}

	stmt := sqlair.MustPrepare(`
SELECT charm_config_type.* AS &charmConfigType.* FROM charm_config_type ORDER BY id;
`, charmConfigType{})

	var results []charmConfigType
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt).GetAll(&results)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 5)

	m := []charm.OptionType{
		charm.OptionString,
		charm.OptionInt,
		charm.OptionFloat,
		charm.OptionBool,
		charm.OptionSecret,
	}

	for i, value := range m {
		c.Logf("result %d: %#v", i, value)
		result, err := encodeConfigType(value)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(result, tc.DeepEquals, results[i].ID)
	}
}
