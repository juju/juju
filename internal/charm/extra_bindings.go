// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm

import (
	"fmt"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/schema"
)

// ExtraBinding represents an extra bindable endpoint that is not a relation.
type ExtraBinding struct {
	Name string `bson:"name" json:"Name"`
}

// When specified, the "extra-bindings" section in the metadata.yaml
// should have the following format:
//
// extra-bindings:
//
//	"<endpoint-name>":
//	...
//
// Endpoint names are strings and must not match existing relation names from
// the Provides, Requires, or Peers metadata sections. The values beside each
// endpoint name must be left out (i.e. "foo": <anything> is invalid).
var extraBindingsSchema = schema.Map(schema.NonEmptyString("binding name"), schema.Nil(""))

func parseMetaExtraBindings(data interface{}) (map[string]ExtraBinding, error) {
	if data == nil {
		return nil, nil
	}

	bindingsMap := data.(map[interface{}]interface{})
	result := make(map[string]ExtraBinding)
	for name := range bindingsMap {
		stringName := name.(string)
		result[stringName] = ExtraBinding{Name: stringName}
	}

	return result, nil
}

func validateMetaExtraBindings(meta Meta) error {
	extraBindings := meta.ExtraBindings
	if extraBindings == nil {
		return nil
	} else if len(extraBindings) == 0 {
		return fmt.Errorf("extra bindings cannot be empty when specified")
	}

	usedExtraNames := set.NewStrings()
	for name, binding := range extraBindings {
		if binding.Name == "" || name == "" {
			return fmt.Errorf("missing binding name")
		}
		if binding.Name != name {
			return fmt.Errorf("mismatched extra binding name: got %q, expected %q", binding.Name, name)
		}
		usedExtraNames.Add(name)
	}

	usedRelationNames := set.NewStrings()
	for relationName := range meta.CombinedRelations() {
		usedRelationNames.Add(relationName)
	}
	notAllowedNames := usedExtraNames.Intersection(usedRelationNames)
	if !notAllowedNames.IsEmpty() {
		notAllowedList := strings.Join(notAllowedNames.SortedValues(), ", ")
		return fmt.Errorf("relation names (%s) cannot be used in extra bindings", notAllowedList)
	}
	return nil
}
