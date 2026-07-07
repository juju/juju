// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config_test

import (
	"encoding/json"
	stdtesting "testing"

	"github.com/juju/tc"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/environs/config"
)

type SourceSuite struct{}

func TestSourceSuite(t *stdtesting.T) {
	tc.Run(t, &SourceSuite{})
}

func (*SourceSuite) TestAttributeDefaultValuesMarshalPreservesEmptyValues(c *tc.C) {
	values := config.AttributeDefaultValues{
		Default:    config.RequiresPromptsMode,
		Controller: "",
		Regions: []config.RegionDefaultValue{{
			Name:  "dummy-region",
			Value: "dummy-value",
		}},
	}

	jsonBytes, err := json.Marshal(values)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(jsonBytes), tc.Equals,
		`{"controller":"","default":"requires-prompts","regions":[{"name":"dummy-region","value":"dummy-value"}]}`)

	yamlBytes, err := yaml.Marshal(values)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(yamlBytes), tc.Equals, ""+
		"default: requires-prompts\n"+
		"controller: \"\"\n"+
		"regions:\n"+
		"- name: dummy-region\n"+
		"  value: dummy-value\n")
}

func (*SourceSuite) TestAttributeDefaultValuesMarshalOmitsNilValues(c *tc.C) {
	values := config.AttributeDefaultValues{
		Controller: "",
	}

	jsonBytes, err := json.Marshal(values)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(jsonBytes), tc.Equals, `{"controller":""}`)

	yamlBytes, err := yaml.Marshal(values)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(yamlBytes), tc.Equals, "controller: \"\"\n")
}
