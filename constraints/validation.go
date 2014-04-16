// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package constraints

import (
	"fmt"
	"reflect"
	"strings"

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
	// checkConflicts returns an error if the constraints Value contains conflicting attributes.
	checkConflicts(cons Value) error
	// checkUnsupported returns an error if the constraints Value contains unsupported attributes.
	checkUnsupported(cons Value) error
	// Validate returns an error if the given constraints are not valid.
	Validate(cons Value) error
	// Merge merges consB into consA, with any conflicting attributes from consB
	// overriding those from consA.
	Merge(consA, consB Value) (Value, error)
}

type notSupportedError struct {
	what []string
}

// NewNotSupportedError returns an error signifying that
// constraint attributes are not supported.
func NewNotSupportedError(what []string) error {
	return &notSupportedError{what: what}
}

func (e *notSupportedError) Error() string {
	return fmt.Sprintf("unsupported constraints: %s", strings.Join(e.what, ","))
}

// IsNotSupportedError reports whether the error
// was created with NewNotSupportedError.
func IsNotSupportedError(err error) bool {
	_, ok := err.(*notSupportedError)
	return ok
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

// checkConflicts is defined on Validator.
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

// checkUnsupported is defined on Validator.
func (v *validator) checkUnsupported(cons Value) error {
	attrNames := cons.hasAny(v.unsupported.Values()...)
	if len(attrNames) > 0 {
		return NewNotSupportedError(attrNames)
	}
	return nil
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
func (v *validator) Validate(cons Value) error {
	// Conflicts are checked first because such errors are fatal.
	if err := v.checkConflicts(cons); err != nil {
		return err
	}
	// Lastly, check for unsupported attributes.
	// Callers might choose to ignore such errors since the
	// constraints value is usable if the unsupported attributes
	// are simply ignored.
	return v.checkUnsupported(cons)
}

// Merge is defined on Validator.
func (v *validator) Merge(consA, consB Value) (Value, error) {
	// First ensure both constraints are valid. We don't care if there
	// are constraint attributes that are unsupported - the caller can
	// either ignore or error as required.
	if err := v.Validate(consA); err != nil && !IsNotSupportedError(err) {
		return Value{}, err
	}
	if err := v.Validate(consB); err != nil && !IsNotSupportedError(err) {
		return Value{}, err
	}
	// Gather any attributes from consA which conflict with those on consB.
	bAttrs := consB.attributesWithValues()
	var aConflicts []string
	for _, bAttr := range bAttrs {
		aConflicts = append(aConflicts, v.conflicts[bAttr].Values()...)
	}
	// Null out the conflicting consA attribute values because
	// consB takes priority. We can't error here because we
	// know that aConflicts contains valid attr names.
	consAMinusConflicts, _ := consA.without(aConflicts...)
	// The result is consB with fallbacks coming from any
	// non conflicting consA attributes.
	return withFallbacks(consB, consAMinusConflicts), nil
}
