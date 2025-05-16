// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config_test

import (
	stdtesting "testing"

	"github.com/juju/collections/set"
	"github.com/juju/schema"
	"github.com/juju/tc"

	"github.com/juju/juju/core/config"
	"github.com/juju/juju/internal/configschema"
	coretesting "github.com/juju/juju/internal/testing"
)

type ConfigSuite struct {
	coretesting.BaseSuite
}

func TestConfigSuite(t *stdtesting.T) { tc.Run(t, &ConfigSuite{}) }

var testFields = configschema.Fields{
	"field1": {
		Description: "field 1 description",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	"field2": {
		Description: "field 2 description",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	"field3": {
		Description: "field 3 description",
		Type:        configschema.Tint,
		Group:       configschema.EnvironGroup,
	},
	"field4": {
		Description: "field 4 description",
		Type:        configschema.Tbool,
		Group:       configschema.EnvironGroup,
	},
	"field5": {
		Description: "field 5 description",
		Type:        configschema.Tattrs,
		Group:       configschema.EnvironGroup,
	},
}

var testDefaults = schema.Defaults{
	"field1": "field 1 default",
	"field3": 42,
}

func (s *ConfigSuite) TestKnownConfigKeys(c *tc.C) {
	c.Assert(config.KnownConfigKeys(
		testFields), tc.DeepEquals, set.NewStrings("field1", "field2", "field3", "field4", "field5"))
}

func (s *ConfigSuite) assertNewConfig(c *tc.C) *config.Config {
	cfg, err := config.NewConfig(
		map[string]interface{}{"field2": "field 2 value", "field4": true, "field5": map[string]interface{}{"a": "b"}},
		testFields, testDefaults)
	c.Assert(err, tc.ErrorIsNil)
	return cfg
}

func (s *ConfigSuite) TestAttributes(c *tc.C) {
	cfg := s.assertNewConfig(c)
	c.Assert(cfg.Attributes(), tc.DeepEquals, config.ConfigAttributes{
		"field1": "field 1 default",
		"field2": "field 2 value",
		"field3": 42,
		"field4": true,
		"field5": map[string]string{"a": "b"},
	})
}

func (s *ConfigSuite) TestNewConfigUnknownAttribute(c *tc.C) {
	_, err := config.NewConfig(map[string]interface{}{"some-attr": "value"}, nil, nil)
	c.Assert(err, tc.ErrorMatches, `unknown key "some-attr" \(value "value"\)`)
}

func (s *ConfigSuite) TestAttributesNil(c *tc.C) {
	cfg := (*config.Config)(nil)
	c.Assert(cfg.Attributes(), tc.IsNil)
}

func (s *ConfigSuite) TestGet(c *tc.C) {
	cfg := s.assertNewConfig(c)
	c.Assert(cfg.Attributes().Get("field1", nil), tc.Equals, "field 1 default")
	c.Assert(cfg.Attributes().Get("missing", "default"), tc.Equals, "default")
}

func (s *ConfigSuite) TestGetString(c *tc.C) {
	cfg := s.assertNewConfig(c)
	c.Assert(cfg.Attributes().GetString("field1", ""), tc.Equals, "field 1 default")
	c.Assert(cfg.Attributes().GetString("missing", "default"), tc.Equals, "default")
}

func (s *ConfigSuite) TestGetInt(c *tc.C) {
	cfg := s.assertNewConfig(c)
	c.Assert(cfg.Attributes().GetInt("field3", -1), tc.Equals, 42)
	c.Assert(cfg.Attributes().GetInt("missing", -1), tc.Equals, -1)
}

func (s *ConfigSuite) TestGetBool(c *tc.C) {
	cfg := s.assertNewConfig(c)
	c.Assert(cfg.Attributes().GetBool("field4", false), tc.Equals, true)
	c.Assert(cfg.Attributes().GetBool("missing", true), tc.Equals, true)
}

func (s *ConfigSuite) TestGetStringMap(c *tc.C) {
	cfg := s.assertNewConfig(c)
	expected := map[string]string{"a": "b"}
	val, err := cfg.Attributes().GetStringMap("field5", nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(val, tc.DeepEquals, expected)
	val, err = cfg.Attributes().GetStringMap("missing", expected)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(val, tc.DeepEquals, expected)
}

func (s *ConfigSuite) TestInvalidStringMap(c *tc.C) {
	cfg := s.assertNewConfig(c)
	_, err := cfg.Attributes().GetStringMap("field1", nil)
	c.Assert(err, tc.ErrorMatches, "string map value of type string not valid")
}
