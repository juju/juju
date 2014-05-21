// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package constraints

import (
	"fmt"
	"math"
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

	// RegisterVocabulary records allowed values for the specified constraint attribute.
	// allowedValues is expected to be a slice/array but is declared as interface{} so
	// that vocabs of different types can be passed in.
	RegisterVocabulary(attributeName string, allowedValues interface{})

	// Validate returns an error if the given constraints are not valid, and also
	// any unsupported attributes.
	Validate(cons Value) ([]string, error)

	// Merge merges cons into consFallback, with any conflicting attributes from cons
	// overriding those from consFallback.
	Merge(consFallback, cons Value) (Value, error)
}

// NewValidator returns a new constraints Validator instance.
func NewValidator() Validator {
	return &validator{
		conflicts: make(map[string]set.Strings),
		vocab:     make(map[string][]interface{}),
	}
}

type validator struct {
	unsupported set.Strings
	conflicts   map[string]set.Strings
	vocab       map[string][]interface{}
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

// RegisterVocabulary is defined on Validator.
func (v *validator) RegisterVocabulary(attributeName string, allowedValues interface{}) {
	k := reflect.TypeOf(allowedValues).Kind()
	if k != reflect.Slice && k != reflect.Array {
		panic(fmt.Errorf("invalid vocab: %v of type %T is not a slice", allowedValues, allowedValues))
	}
	// Convert the vocab to a slice of interface{}
	var allowedSlice []interface{}
	val := reflect.ValueOf(allowedValues)
	for i := 0; i < val.Len(); i++ {
		allowedSlice = append(allowedSlice, val.Index(i).Interface())
	}
	v.vocab[attributeName] = allowedSlice
}

// checkConflicts returns an error if the constraints Value contains conflicting attributes.
func (v *validator) checkConflicts(cons Value) error {
	attrValues := cons.attributesWithValues()
	attrSet := set.NewStrings()
	for attrTag := range attrValues {
		attrSet.Add(attrTag)
	}
	for _, attrTag := range attrSet.SortedValues() {
		conflicts, ok := v.conflicts[attrTag]
		if !ok {
			continue
		}
		for _, conflict := range conflicts.SortedValues() {
			if attrSet.Contains(conflict) {
				return fmt.Errorf("ambiguous constraints: %q overlaps with %q", attrTag, conflict)
			}
		}
	}
	return nil
}

// checkUnsupported returns any unsupported attributes.
func (v *validator) checkUnsupported(cons Value) []string {
	return cons.hasAny(v.unsupported.Values()...)
}

// checkValidValues returns an error if the constraints value contains an
// attribute value which is not allowed by the vocab which may have been
// registered for it.
func (v *validator) checkValidValues(cons Value) error {
	for attrTag, attrValue := range cons.attributesWithValues() {
		k := reflect.TypeOf(attrValue).Kind()
		if k == reflect.Slice || k == reflect.Array {
			// For slices we check that all values are valid.
			val := reflect.ValueOf(attrValue)
			for i := 0; i < val.Len(); i++ {
				if err := v.checkInVocab(attrTag, val.Index(i).Interface()); err != nil {
					return err
				}
			}
		} else {
			if err := v.checkInVocab(attrTag, attrValue); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkInVocab returns an error if the attribute value is not allowed by the
// vocab which may have been registered for it.
func (v *validator) checkInVocab(attributeName string, attributeValue interface{}) error {
	validValues, ok := v.vocab[attributeName]
	if !ok {
		return nil
	}
	for _, validValue := range validValues {
		if coerce(validValue) == coerce(attributeValue) {
			return nil
		}
	}
	return fmt.Errorf(
		"invalid constraint value: %v=%v\nvalid values are: %v", attributeName, attributeValue, validValues)
}

// coerce returns v in a format that allows constraint values to be easily compared.
// Its main purpose is to cast all numeric values to int64 or float64.
func coerce(v interface{}) interface{} {
	if v != nil {
		switch vv := reflect.TypeOf(v); vv.Kind() {
		case reflect.String:
			return v
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return int64(reflect.ValueOf(v).Int())
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			uval := reflect.ValueOf(v).Uint()
			// Just double check the value is in range.
			if uval > math.MaxInt64 {
				panic(fmt.Errorf("constraint value %v is too large", uval))
			}
			return int64(uval)
		case reflect.Float32, reflect.Float64:
			return float64(reflect.ValueOf(v).Float())
		}
	}
	return v
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
	if err := v.checkValidValues(cons); err != nil {
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
	attrValues := cons.attributesWithValues()
	var fallbackConflicts []string
	for attrTag := range attrValues {
		fallbackConflicts = append(fallbackConflicts, v.conflicts[attrTag].Values()...)
	}
	// Null out the conflicting consFallback attribute values because
	// cons takes priority. We can't error here because we
	// know that aConflicts contains valid attr names.
	consFallbackMinusConflicts, _ := consFallback.without(fallbackConflicts...)
	// The result is cons with fallbacks coming from any
	// non conflicting consFallback attributes.
	return withFallbacks(cons, consFallbackMinusConflicts), nil
}
