// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package constraints

import (
	"fmt"
	"reflect"

	"launchpad.net/juju-core/utils/set"
)

// Validator defines operations on constraints attributes which are
// used to ensure a constraints value is valid, as well as being able
// to handle overridden attributes.
type Validator interface {

	// RegisterConflicts is used to define cross-constraint override behaviour.
	// The red and blue attribute lists contain attribute names which conflict
	// with those in the other list.
	// When two constraints conflict:
	//  it is an error to set both constraints in the same constraints Value.
	//  when a constraints Value overrides another which specifies a conflicting
	//   attribute, the attribute in the overridden Value is cleared.
	RegisterConflicts(reds, blues []string)

	// RegisterUnsupported records attributes which are not supported by a constraints Value.
	RegisterUnsupported(unsupported []string)

	// Validate returns an error if the given constraints are not valid, and also
	// any unsupported attributes.
	Validate(cons Value) ([]string, error)

	// Merge merges cons into consFallback, with any conflicting attributes from cons
	// overriding those from consFallback.
	Merge(consFallback, cons Value) (Value, error)
}

// NewValidator returns a new constraints Validator instance.
func NewValidator() Validator {
	c := validator{}
	c.conflicts = make(map[string]set.Strings)
	return &c
}

type validator struct {
	unsupported set.Strings
	conflicts   map[string]set.Strings
}

// RegisterConflicts is defined on Validator.
func (v *validator) RegisterConflicts(reds, blues []string) {
	for _, red := range reds {
		v.conflicts[red] = set.NewStrings(blues...)
	}
	for _, blue := range blues {
		v.conflicts[blue] = set.NewStrings(reds...)
	}
}

// RegisterUnsupported is defined on Validator.
func (v *validator) RegisterUnsupported(unsupported []string) {
	v.unsupported = set.NewStrings(unsupported...)
}

// checkConflicts returns an error if the constraints Value contains conflicting attributes.
func (v *validator) checkConflicts(cons Value) error {
	attrNames := cons.attributesWithValues()
	attrSet := set.NewStrings(attrNames...)
	for _, attr := range attrNames {
		conflicts, ok := v.conflicts[attr]
		if !ok {
			continue
		}
		for _, conflict := range conflicts.Values() {
			if attrSet.Contains(conflict) {
				return fmt.Errorf("ambiguous constraints: %q overlaps with %q", attr, conflict)
			}
		}
	}
	return nil
}

// checkUnsupported returns any unsupported attributes.
func (v *validator) checkUnsupported(cons Value) []string {
	return cons.hasAny(v.unsupported.Values()...)
}

// withFallbacks returns a copy of v with nil values taken from vFallback.
func withFallbacks(v Value, vFallback Value) Value {
	result := vFallback
	for _, fieldName := range fieldNames {
		resultVal := reflect.ValueOf(&result).Elem().FieldByName(fieldName)
		val := reflect.ValueOf(&v).Elem().FieldByName(fieldName)
		if !val.IsNil() {
			resultVal.Set(val)
		}
	}
	return result
}

// Validate is defined on Validator.
func (v *validator) Validate(cons Value) ([]string, error) {
	unsupported := v.checkUnsupported(cons)
	if err := v.checkConflicts(cons); err != nil {
		return unsupported, err
	}
	return unsupported, nil
}

// Merge is defined on Validator.
func (v *validator) Merge(consFallback, cons Value) (Value, error) {
	// First ensure both constraints are valid. We don't care if there
	// are constraint attributes that are unsupported.
	if _, err := v.Validate(consFallback); err != nil {
		return Value{}, err
	}
	if _, err := v.Validate(cons); err != nil {
		return Value{}, err
	}
	// Gather any attributes from consFallback which conflict with those on cons.
	attrs := cons.attributesWithValues()
	var fallbackConflicts []string
	for _, attr := range attrs {
		fallbackConflicts = append(fallbackConflicts, v.conflicts[attr].Values()...)
	}
	// Null out the conflicting consFallback attribute values because
	// cons takes priority. We can't error here because we
	// know that aConflicts contains valid attr names.
	consFallbackMinusConflicts, _ := consFallback.without(fallbackConflicts...)
	// The result is cons with fallbacks coming from any
	// non conflicting consFallback attributes.
	return withFallbacks(cons, consFallbackMinusConflicts), nil
}
