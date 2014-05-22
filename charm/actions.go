// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"fmt"
	"io"
	"io/ioutil"
	"regexp"

	"github.com/xeipuuv/gojsonschema"
	"launchpad.net/goyaml"
)

// Actions defines the available actions for the charm.
type Actions struct {
	ActionSpecs map[string]ActionSpec `yaml:"actions"`
}

// ActionSpec is a definition of the parameters and traits of an Action.
type ActionSpec struct {
	Description string
	Params      map[string]interface{}
}

// ReadActions builds an Actions spec from a charm's actions.yaml.
func ReadActionsYaml(r io.Reader) (*Actions, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var unmarshaledActions Actions
	if err := goyaml.Unmarshal(data, &unmarshaledActions); err != nil {
		return nil, err
	}

	nameRule := regexp.MustCompile("^[^-][a-z-]+[^-]$")

	for name, actionSpec := range unmarshaledActions.ActionSpecs {
		badName := !nameRule.MatchString(name)
		if badName {
			return nil, fmt.Errorf("bad action name %s", name)
		}
		_, err := gojsonschema.NewJsonSchemaDocument(actionSpec.Params)
		if err != nil {
			return nil, fmt.Errorf("invalid params schema for action %q: %v", err)
		}
		for paramName, _ := range unmarshaledActions.ActionSpecs[name].Params {
			badParam := !nameRule.MatchString(paramName)
			if badParam {
				return nil, fmt.Errorf("bad param name %s", paramName)
			}
		}
	}
	return &unmarshaledActions, nil
}
