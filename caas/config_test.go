// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas_test

import (
	"github.com/juju/schema"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/testing"
)

var baseFields = environschema.Fields{
	caas.JujuExternalHostNameKey: {
		Description: "the external hostname of an exposed application",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	caas.JujuApplicationPath: {
		Description: "the relative http path used to access an application",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
}

var baseDefaults = schema.Defaults{
	caas.JujuApplicationPath: "/",
}

type ConfigSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&ConfigSuite{})

func (s *ConfigSuite) TestConfigSchemaNoProviderFields(c *gc.C) {
	fields, err := caas.ConfigSchema(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fields, jc.DeepEquals, baseFields)
}

func (s *ConfigSuite) TestConfigSchemaProviderFields(c *gc.C) {
	extraFields := environschema.Fields{
		"extra": {
			Description: "some field",
			Type:        environschema.Tstring,
		},
	}
	fields, err := caas.ConfigSchema(extraFields)
	c.Assert(err, jc.ErrorIsNil)

	expectedFields := make(environschema.Fields)
	for name, f := range baseFields {
		expectedFields[name] = f
	}
	for name, f := range extraFields {
		expectedFields[name] = f
	}
	c.Assert(fields, jc.DeepEquals, expectedFields)
}

func (s *ConfigSuite) TestConfigSchemaProviderFieldsConflict(c *gc.C) {
	extraFields := environschema.Fields{
		"juju-external-hostname": {
			Description: "some field",
			Type:        environschema.Tstring,
		},
	}
	_, err := caas.ConfigSchema(extraFields)
	c.Assert(err, gc.ErrorMatches, `config field "juju-external-hostname" clashes with common config`)
}

func (s *ConfigSuite) TestConfigDefaultsNoProviderDefaults(c *gc.C) {
	defaults := caas.ConfigDefaults(nil)
	c.Assert(defaults, jc.DeepEquals, baseDefaults)
}

func (s *ConfigSuite) TestConfigSchemaProviderDefaults(c *gc.C) {
	extraDefaults := schema.Defaults{
		"extra": "extra default",
	}
	defaults := caas.ConfigDefaults(extraDefaults)

	expectedDefaults := make(schema.Defaults)
	for name, d := range baseDefaults {
		expectedDefaults[name] = d
	}
	for name, d := range defaults {
		expectedDefaults[name] = d
	}
	c.Assert(defaults, jc.DeepEquals, expectedDefaults)
}
