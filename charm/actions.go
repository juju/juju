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

const (
	// charm/json-schema-v4-draft.json
	SCHEMA_VERSION = "file://json-schema-v4-draft.json"
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

		// Make sure the parameters are acceptable.
		cleansedParams := make(map[string]interface{})
		for paramName, param := range actionSpec.Params {
			if valid := paramNameRule.MatchString(paramName); !valid {
				return nil, fmt.Errorf("bad param name %s", paramName)
			}

			// Clean any map[interface{}]interface{}s out so they don't
			// cause problems with BSON serialization later.
			cleansedParam, err := cleanse(param)
			if err != nil {
				return nil, err
			}
			cleansedParams[paramName] = cleansedParam
		}

		// // Make sure the returned value coerces properly
		// cleansedTypedParams, ok := cleansedParams.(map[string]interface{})
		// if !ok {
		// 	return nil, fmt.Errorf("cleansed map not a map[string]interface{}")
		// }

		// Now substitute the cleansed map into the original.
		var swap = unmarshaledActions.ActionSpecs[name]
		swap.Params = cleansedParams
		unmarshaledActions.ActionSpecs[name] = swap

		// Make sure the new Params doc can be loaded as a JSON-Schema
		// document.
		_, err = gojsonschema.NewJsonSchemaDocument(unmarshaledActions.ActionSpecs[name].Params)
		if err != nil {
			return nil, fmt.Errorf("invalid params schema for action %s: %v", name, err)
		}

		// Make sure the new Params doc conforms to JSON-Schema
		// Draft 4 (http://json-schema.org/latest/json-schema-core.html)
		jsonSchemaDefinition, err := gojsonschema.NewJsonSchemaDocument(SCHEMA_VERSION)
		if err != nil {
			return nil, fmt.Errorf("invalid json-schema at %s: %v", SCHEMA_VERSION, err)
		}

		validationResults := jsonSchemaDefinition.Validate(cleansedParams)

		if !validationResults.Valid() {
			errorStrings := make([]string, 0)
			for i, schemaError := range validationResults.Errors() {
				errorStrings = append(errorStrings, "json-schema error "+string(i)+": "+schemaError.String())
			}
			return nil, fmt.Errorf("Invalid params schema for action %s: %v", name, errorStrings)
		}
	}
	return &unmarshaledActions, nil
}

func cleanse(input interface{}) (interface{}, error) {
	switch typedInput := input.(type) {

	// In this case, recurse in.
	case map[string]interface{}:
		newMap := make(map[string]interface{})
		for key, value := range typedInput {
			newValue, err := cleanse(value)
			if err != nil {
				return nil, err
			}
			newMap[key] = newValue
		}
		return newMap, nil

	// Coerce keys to strings and error out if there's a problem; then recurse.
	case map[interface{}]interface{}:
		newMap := make(map[string]interface{})
		for key, value := range typedInput {
			typedKey, ok := key.(string)
			if !ok {
				return nil, errors.New("map keyed with non-string value")
			}
			newMapValue, err := cleanse(value)
			if err != nil {
				return nil, err
			}
			newMap[typedKey] = newMapValue
		}
		return newMap, nil

	// Recurse
	case []interface{}:
		newSlice := make([]interface{}, 0)
		for _, sliceValue := range typedInput {
			newSliceValue, err := cleanse(sliceValue)
			if err != nil {
				return nil, errors.New("map keyed with non-string value")
			}
			newSlice = append(newSlice, newSliceValue)
		}
		return newSlice, nil

	// Other kinds of values are OK.
	default:
		return input, nil
	}
}
