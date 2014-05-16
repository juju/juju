// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"fmt"
	"io"
	"io/ioutil"
	"reflect"

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

// ReadActions reads an Actions from YAML.
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
		return nil, fmt.Errorf("Empty actions definition")
	}
	if reflect.DeepEqual(actionsSpec, &Actions{}) {
		return nil, fmt.Errorf("actions.yaml failed to unmarshal -- key mismatch")
	}
	for _, actionSpec := range actionsSpec.ActionSpecs {
		_, err := gojsonschema.NewJsonSchemaDocument(actionSpec.Params)
		if err != nil {
			return nil, fmt.Errorf("error loading Charm actions.yaml -- nonconformant params: %v", err)
		}
	}
	return actionsSpec, nil
}

// actionSpec returns the named ActionSpec from the Actions, or an error if none
// such exists.
func (a *Actions) actionSpec(name string) (ActionSpec, error) {
	if actionSpec, ok := a.ActionSpecs[name]; ok {
		return actionSpec, nil
	}
	return ActionSpec{}, fmt.Errorf("unknown action %q", name)
}
