// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config_test

import (
	"github.com/juju/collections/set"
	"github.com/juju/schema"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/core/config"
	coretesting "github.com/juju/juju/testing"
)

type ConfigSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&ConfigSuite{})

var testFields = environschema.Fields{
	"field1": {
		Description: "field 1 description",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	"field2": {
		Description: "field 2 description",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	"field3": {
		Description: "field 3 description",
		Type:        environschema.Tint,
		Group:       environschema.EnvironGroup,
	},
	"field4": {
		Description: "field 4 description",
		Type:        environschema.Tbool,
		Group:       environschema.EnvironGroup,
	},
	"field5": {
		Description: "field 5 description",
		Type:        environschema.Tattrs,
		Group:       environschema.EnvironGroup,
	},
}

var testDefaults = schema.Defaults{
	"field1": "field 1 default",
	"field3": 42,
}

func (s *ConfigSuite) TestKnownConfigKeys(c *gc.C) {
	c.Assert(config.KnownConfigKeys(
		testFields), gc.DeepEquals, set.NewStrings("field1", "field2", "field3", "field4", "field5"))
}

func (s *ConfigSuite) assertNewConfig(c *gc.C) *config.Config {
	cfg, err := config.NewConfig(
		map[string]interface{}{"field2": "field 2 value", "field4": true, "field5": map[string]interface{}{"a": "b"}},
		testFields, testDefaults)
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}

func (s *ConfigSuite) TestAttributes(c *gc.C) {
	cfg := s.assertNewConfig(c)
	c.Assert(cfg.Attributes(), jc.DeepEquals, config.ConfigAttributes{
		"field1": "field 1 default",
		"field2": "field 2 value",
		"field3": 42,
		"field4": true,
		"field5": map[string]string{"a": "b"},
	})
}

func (s *ConfigSuite) TestNewConfigUnknownAttribute(c *gc.C) {
	_, err := config.NewConfig(map[string]interface{}{"some-attr": "value"}, nil, nil)
	c.Assert(err, gc.ErrorMatches, `unknown key "some-attr" \(value "value"\)`)
}

func (s *ConfigSuite) TestAttributesNil(c *gc.C) {
	cfg := (*config.Config)(nil)
	c.Assert(cfg.Attributes(), gc.IsNil)
}

func (s *ConfigSuite) TestGet(c *gc.C) {
	cfg := s.assertNewConfig(c)
	c.Assert(cfg.Attributes().Get("field1", nil), gc.Equals, "field 1 default")
	c.Assert(cfg.Attributes().Get("missing", "default"), gc.Equals, "default")
}

func (s *ConfigSuite) TestGetString(c *gc.C) {
	cfg := s.assertNewConfig(c)
	c.Assert(cfg.Attributes().GetString("field1", ""), gc.Equals, "field 1 default")
	c.Assert(cfg.Attributes().GetString("missing", "default"), gc.Equals, "default")
}

func (s *ConfigSuite) TestGetInt(c *gc.C) {
	cfg := s.assertNewConfig(c)
	c.Assert(cfg.Attributes().GetInt("field3", -1), gc.Equals, 42)
	c.Assert(cfg.Attributes().GetInt("missing", -1), gc.Equals, -1)
}

func (s *ConfigSuite) TestGetBool(c *gc.C) {
	cfg := s.assertNewConfig(c)
	c.Assert(cfg.Attributes().GetBool("field4", false), gc.Equals, true)
	c.Assert(cfg.Attributes().GetBool("missing", true), gc.Equals, true)
}

func (s *ConfigSuite) TestGetStringMap(c *gc.C) {
	cfg := s.assertNewConfig(c)
	expected := map[string]string{"a": "b"}
	val, err := cfg.Attributes().GetStringMap("field5", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(val, jc.DeepEquals, expected)
	val, err = cfg.Attributes().GetStringMap("missing", expected)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(val, jc.DeepEquals, expected)
}

func (s *ConfigSuite) TestInvalidStringMap(c *gc.C) {
	cfg := s.assertNewConfig(c)
	_, err := cfg.Attributes().GetStringMap("field1", nil)
	c.Assert(err, gc.ErrorMatches, "string map value of type string not valid")
}
