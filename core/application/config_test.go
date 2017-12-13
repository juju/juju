// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"github.com/juju/schema"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/core/application"
	coretesting "github.com/juju/juju/testing"
)

type ApplicationSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&ApplicationSuite{})

var baseFields = environschema.Fields{
	application.JujuExternalHostNameKey: {
		Description: "the external hostname of an exposed application",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	application.JujuApplicationPath: {
		Description: "the relative http path used to access an application",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
}

func (s *ApplicationSuite) TestConfigSchemaNoExtra(c *gc.C) {
	fields, err := application.ConfigSchema(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fields, jc.DeepEquals, application.ConfigFields(baseFields))
}

func (s *ApplicationSuite) TestConfigSchemaExtra(c *gc.C) {
	extraFields := environschema.Fields{
		"extra": {
			Description: "some field",
			Type:        environschema.Tstring,
		},
	}
	fields, err := application.ConfigSchema(extraFields)
	c.Assert(err, jc.ErrorIsNil)

	expectedFields := make(application.ConfigFields)
	for name, f := range baseFields {
		expectedFields[name] = f
	}
	for name, f := range extraFields {
		expectedFields[name] = f
	}
	c.Assert(fields, jc.DeepEquals, expectedFields)
}

func (s *ApplicationSuite) TestConfigSchemaKnownConfigKeys(c *gc.C) {
	fields, err := application.ConfigSchema(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fields.KnownConfigKeys(),
		gc.DeepEquals, set.NewStrings([]string{"juju-external-hostname", "juju-application-path"}...))
}

func (s *ApplicationSuite) TestNewConfigUnknownAttribute(c *gc.C) {
	_, err := application.NewConfig(map[string]interface{}{"some-attr": "value"}, nil, nil)
	c.Assert(err, gc.ErrorMatches, `unknown key "some-attr" \(value "value"\)`)
}

func (s *ApplicationSuite) TestAttributes(c *gc.C) {
	cfg, err := application.NewConfig(
		map[string]interface{}{"juju-external-hostname": "value", "juju-application-path": "path"}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.Attributes(), jc.DeepEquals, application.ConfigAttributes{
		"juju-external-hostname": "value",
		"juju-application-path":  "path"})
}

func (s *ApplicationSuite) TestAttributesNil(c *gc.C) {
	cfg := (*application.Config)(nil)
	c.Assert(cfg.Attributes(), gc.IsNil)
}

func (s *ApplicationSuite) TestAttributeWithDefault(c *gc.C) {
	cfg, err := application.NewConfig(nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.Attributes(), jc.DeepEquals, application.ConfigAttributes{"juju-application-path": "/"})
}

func (s *ApplicationSuite) TestExtraAttributes(c *gc.C) {
	extraFields := environschema.Fields{
		"extra": {
			Description: "some field",
			Type:        environschema.Tstring,
		},
		"extra2": {
			Description: "some field",
			Type:        environschema.Tstring,
		},
	}
	extraDefaults := schema.Defaults{
		"extra": "fred",
	}
	cfg, err := application.NewConfig(nil, extraFields, extraDefaults)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.Attributes(), jc.DeepEquals, application.ConfigAttributes{
		"juju-application-path": "/",
		"extra":                 "fred",
	})
}

func (s *ApplicationSuite) TestGet(c *gc.C) {
	cfg, err := application.NewConfig(map[string]interface{}{"juju-external-hostname": "ext-host"}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.Get("juju-external-hostname"), gc.Equals, "ext-host")
	c.Assert(cfg.Get("missing"), gc.IsNil)
}

func (s *ApplicationSuite) TestGetString(c *gc.C) {
	cfg, err := application.NewConfig(map[string]interface{}{"juju-external-hostname": "ext-host"}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.GetString("juju-external-hostname"), gc.Equals, "ext-host")
	c.Assert(cfg.GetString("missing"), gc.Equals, "")
}

func (s *ApplicationSuite) TestGetInt(c *gc.C) {
	extraFields := environschema.Fields{
		"extra": {
			Description: "some field",
			Type:        environschema.Tint,
		},
	}
	cfg, err := application.NewConfig(map[string]interface{}{"extra": 456}, extraFields, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.GetInt("extra"), gc.Equals, 456)
	c.Assert(cfg.GetInt("missing"), gc.Equals, 0)
}

func (s *ApplicationSuite) TestGetBool(c *gc.C) {
	extraFields := environschema.Fields{
		"extra": {
			Description: "some field",
			Type:        environschema.Tbool,
		},
	}
	cfg, err := application.NewConfig(map[string]interface{}{"extra": true}, extraFields, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.GetBool("extra"), gc.Equals, true)
	c.Assert(cfg.GetBool("missing"), gc.Equals, false)
}
