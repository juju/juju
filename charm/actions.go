// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"fmt"
	"io"
	"io/ioutil"
	"reflect"
	"regexp"
	"github.com/xeipuuv/gojsonschema"
	"launchpad.net/goyaml"
)

// Actions defines the available actions for the charm.
type Actions struct {
	ActionSpecs map[string]ActionSpec
}

// ActionSpec is a definition of the parameters and traits of an Action.
type ActionSpec struct {
	Description string
	Params      map[string]interface{}
}

// NewActions returns a new Actions object without any defined content.
func NewActions() *Actions {
	return &Actions{map[string]ActionSpec{}}
}

// ReadActions builds an Actions spec from a charm's actions.yaml.
func ReadActionsYaml(r io.Reader) (*Actions, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var actionsSpec *Actions
	if err := goyaml.Unmarshal(data, &actionsSpec); err != nil {
		return nil, err
	}
	if actionsSpec == nil {
		return nil, fmt.Errorf("empty actions definition")
	}
	if data != []byte{} && reflect.DeepEqual(actionsSpec, &Actions{}) {
		return nil, fmt.Errorf("actions.yaml failed to unmarshal -- key mismatch")
	}

	nameRule := regexp.MustCompile("^[^-][a-z-]+[^-]$")

	for name, actionSpec := range actionsSpec.ActionSpecs {
		badName := !nameRule.MatchString(name)
		if badName {
			return nil, fmt.Errorf("bad action name %s", name)
		}
		_, err := gojsonschema.NewJsonSchemaDocument(actionSpec.Params)
		if err != nil {
			return nil, fmt.Errorf("invalid params schema for action %q: %v", err)
		}
	}
	return actionsSpec, nil
}
