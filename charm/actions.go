// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"fmt"
	"io"
	"io/ioutil"
	"regexp"

	"github.com/binary132/gojsonschema"
	"launchpad.net/goyaml"
)

var actionNameRule = regexp.MustCompile("^[a-z](?:[a-z-]*[a-z])?$")
var paramNameRule = regexp.MustCompile("^[a-z$](?:[a-z-]*[a-z])?$")

// Actions defines the available actions for the charm.  Additional params
// may be added as metadata at a future time (e.g. version.)
type Actions struct {
	ActionSpecs map[string]ActionSpec `yaml:"actions"`
}

// ActionSpec is a definition of the parameters and traits of an Action.
// The Params map is expected to conform to JSON-Schema Draft 4 as defined at
// http://json-schema.org/draft-04/schema# (see http://json-schema.org/latest/json-schema-core.html)
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

	for name, actionSpec := range unmarshaledActions.ActionSpecs {
		if valid := actionNameRule.MatchString(name); !valid {
			return nil, fmt.Errorf("bad action name %s", name)
		}
		_, err := gojsonschema.NewJsonSchemaDocument(actionSpec.Params)
		if err != nil {
			return nil, fmt.Errorf("invalid params schema for action schema %s: %v", name, err)
		}

		for paramName, _ := range unmarshaledActions.ActionSpecs[name].Params {
			if valid := paramNameRule.MatchString(paramName); !valid {
				return nil, fmt.Errorf("bad param name %s", paramName)
			}
		}
	}
	return &unmarshaledActions, nil
}
