// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"github.com/juju/collections/set"
	"github.com/juju/schema"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/core/application"
	coretesting "github.com/juju/juju/testing"
)

type ApplicationSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&ApplicationSuite{})

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

func (s *ApplicationSuite) TestKnownConfigKeys(c *gc.C) {
	c.Assert(application.KnownConfigKeys(
		testFields), gc.DeepEquals, set.NewStrings("field1", "field2", "field3", "field4", "field5"))
}

func (s *ApplicationSuite) assertNewConfig(c *gc.C) *application.Config {
	cfg, err := application.NewConfig(
		map[string]interface{}{"field2": "field 2 value", "field4": true, "field5": map[string]interface{}{"a": "b"}},
		testFields, testDefaults)
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}

func (s *ApplicationSuite) TestAttributes(c *gc.C) {
	cfg := s.assertNewConfig(c)
	c.Assert(cfg.Attributes(), jc.DeepEquals, application.ConfigAttributes{
		"field1": "field 1 default",
		"field2": "field 2 value",
		"field3": 42,
		"field4": true,
		"field5": map[string]string{"a": "b"},
	})
}

func (s *ApplicationSuite) TestNewConfigUnknownAttribute(c *gc.C) {
	_, err := application.NewConfig(map[string]interface{}{"some-attr": "value"}, nil, nil)
	c.Assert(err, gc.ErrorMatches, `unknown key "some-attr" \(value "value"\)`)
}

func (s *ApplicationSuite) TestAttributesNil(c *gc.C) {
	cfg := (*application.Config)(nil)
	c.Assert(cfg.Attributes(), gc.IsNil)
}

func (s *ApplicationSuite) TestGet(c *gc.C) {
	cfg := s.assertNewConfig(c)
	c.Assert(cfg.Attributes().Get("field1", nil), gc.Equals, "field 1 default")
	c.Assert(cfg.Attributes().Get("missing", "default"), gc.Equals, "default")
}

func (s *ApplicationSuite) TestGetString(c *gc.C) {
	cfg := s.assertNewConfig(c)
	c.Assert(cfg.Attributes().GetString("field1", ""), gc.Equals, "field 1 default")
	c.Assert(cfg.Attributes().GetString("missing", "default"), gc.Equals, "default")
}

func (s *ApplicationSuite) TestGetInt(c *gc.C) {
	cfg := s.assertNewConfig(c)
	c.Assert(cfg.Attributes().GetInt("field3", -1), gc.Equals, 42)
	c.Assert(cfg.Attributes().GetInt("missing", -1), gc.Equals, -1)
}

func (s *ApplicationSuite) TestGetBool(c *gc.C) {
	cfg := s.assertNewConfig(c)
	c.Assert(cfg.Attributes().GetBool("field4", false), gc.Equals, true)
	c.Assert(cfg.Attributes().GetBool("missing", true), gc.Equals, true)
}

func (s *ApplicationSuite) TestGetStringMap(c *gc.C) {
	cfg := s.assertNewConfig(c)
	expected := map[string]string{"a": "b"}
	val, err := cfg.Attributes().GetStringMap("field5", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(val, jc.DeepEquals, expected)
	val, err = cfg.Attributes().GetStringMap("missing", expected)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(val, jc.DeepEquals, expected)
}

func (s *ApplicationSuite) TestInvalidStringMap(c *gc.C) {
	cfg := s.assertNewConfig(c)
	_, err := cfg.Attributes().GetStringMap("field1", nil)
	c.Assert(err, gc.ErrorMatches, "string map value of type string not valid")
}
