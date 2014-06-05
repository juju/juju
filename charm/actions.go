// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"errors"
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
	ActionSpecs map[string]ActionSpec `yaml:"actions,omitempty" bson:",omitempty"`
}

// ActionSpec is a definition of the parameters and traits of an Action.
// The Params map is expected to conform to JSON-Schema Draft 4 as defined at
// http://json-schema.org/draft-04/schema# (see http://json-schema.org/latest/json-schema-core.html)
type ActionSpec struct {
	Description string
	Params      map[string]interface{}
}

func NewActions() *Actions {
	return &Actions{}
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

		// Make sure the names of parameters are acceptable.
		for paramName, _ := range unmarshaledActions.ActionSpecs[name].Params {
			if valid := paramNameRule.MatchString(paramName); !valid {
				return nil, fmt.Errorf("bad param name %s", paramName)
			}
		}

		// Clean any map[interface{}]interface{}s out so they don't
		// cause problems with BSON serialization later.
		cleanedParams, err := cleanseStringMap(actionSpec.Params)
		if err != nil {
			return nil, fmt.Errorf("invalid params schema for action schema %s: %v", name, err)
		}

		// Now substitute the cleaned value into the original.
		var swap = unmarshaledActions.ActionSpecs[name]
		swap.Params = cleanedParams
		unmarshaledActions.ActionSpecs[name] = swap

		// Make sure the new Params doc conforms to JSON-Schema
		// Draft 4 (http://json-schema.org/latest/json-schema-core.html)
		_, err = gojsonschema.NewJsonSchemaDocument(actionSpec.Params)
		if err != nil {
			return nil, fmt.Errorf("invalid params schema for action schema %s: %v", name, err)
		}

	}
	return &unmarshaledActions, nil
}

func cleanseMap(input map[interface{}]interface{}) (map[string]interface{}, error) {
	output := make(map[string]interface{})

	for inKey, inValue := range input {
		typedKey, ok := inKey.(string)
		if !ok {
			return nil, errors.New("map keyed with non-string value")
		}
		output[typedKey] = inValue
	}

	return cleanseStringMap(output)
}

func cleanseSlice(input []interface{}) ([]interface{}, error) {
	output := make([]interface{}, 0)

	for _, sliceVal := range input {
		switch typedSliceVal := sliceVal.(type) {

		case map[string]interface{}:
			newSliceVal, err := cleanseStringMap(typedSliceVal)
			if err != nil {
				return nil, err
			}
			output = append(output, newSliceVal)

		// Rebuild inner map[interface{}]interface{}s
		// using recursive technique.
		case map[interface{}]interface{}:
			newSliceVal, err := cleanseMap(typedSliceVal)
			if err != nil {
				return nil, err
			}
			output = append(output, newSliceVal)

		case []interface{}:
			newSliceVal, err := cleanseSlice(typedSliceVal)
			if err != nil {
				return nil, err
			}
			output = append(output, newSliceVal)

		// Otherwise, just use the same value
		default:
			output = append(output, sliceVal)
		}
	}
	return output, nil
}

// stripBadInterfaces recurses through the values of a map[string]interface{}
// and attempts to build a copy where any map[interface{}]interface{}s in the
// original are rebuilt as map[string]interface{}s.  If any inner maps have
// keys which are not of type string, the function will fail with an error.
// This function does not mutate the original map.
func cleanseStringMap(input map[string]interface{}) (map[string]interface{}, error) {
	output := make(map[string]interface{})

	for key, val := range input {
		switch typedVal := val.(type) {
		// If the value is already a map[string]interface{}, recurse
		// into it and return the conformed version or error.
		case map[string]interface{}:
			newValue, err := cleanseStringMap(typedVal)
			if err != nil {
				return nil, err
			}
			output[key] = newValue

		// If the value is a map[interface{}]interface{}, check that
		// its keys are strings and build a new map[string]interface{}
		// using the typed keys, and same values.  Then recurse on
		// the new map, returning the conformed version or error.
		case map[interface{}]interface{}:
			cleansedVal, err := cleanseMap(typedVal)
			if err != nil {
				output[key] = cleansedVal
			}

		// If the value is an interface{} slice, step through it and
		// recursively rebuild any map[interface{}]interface{}s in it.
		case []interface{}:
			cleansedVal, err := cleanseSlice(typedVal)
			if err != nil {
				output[key] = cleansedVal
			}

		// If the value is something else, it's fine to use.
		default:
			output[key] = val
		}
	}

	return output, nil
}
