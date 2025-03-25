// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package constraints

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/juju/collections/set"

	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/stringcompare"
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

	// RegisterConflictResolver defines a resolver between two conflicting constraints.
	// When there is a registered conflict between two contraints, it can be resolved by
	// calling the resolver, if it returns a nil error, the conflict is considered resolved.
	RegisterConflictResolver(red, blue string, resolver ConflictResolver)

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

	// UpdateVocabulary merges new attribute values with existing values.
	// This method does not overwrite or delete values, i.e.
	//     if existing values are {a, b}
	//     and new values are {c, d},
	//     then the merge result would be {a, b, c, d}.
	UpdateVocabulary(attributeName string, newValues interface{})
}

type ConflictResolver func(attrValues map[string]interface{}) error

// NewValidator returns a new constraints Validator instance.
func NewValidator() Validator {
	return &validator{
		conflicts:             make(map[string]set.Strings),
		conflictResolvers:     make(map[string]ConflictResolver),
		vocab:                 make(map[string][]interface{}),
		validValuesCountLimit: 10,
	}
}

type validator struct {
	unsupported           set.Strings
	conflicts             map[string]set.Strings
	conflictResolvers     map[string]ConflictResolver
	vocab                 map[string][]interface{}
	validValuesCountLimit int
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

func conflictResolverId(red, blue string) string {
	idx := []string{red, blue}
	sort.Strings(idx)
	return strings.Join(idx, " ")
}

func (v *validator) RegisterConflictResolver(red, blue string, resolver ConflictResolver) {
	id := conflictResolverId(red, blue)
	v.conflictResolvers[id] = resolver
}

// RegisterUnsupported is defined on Validator.
func (v *validator) RegisterUnsupported(unsupported []string) {
	v.unsupported = set.NewStrings(unsupported...)
}

// RegisterVocabulary is defined on Validator.
func (v *validator) RegisterVocabulary(attributeName string, allowedValues interface{}) {
	v.vocab[resolveAlias(attributeName)] = convertToSlice(allowedValues)
}

var checkIsCollection = func(coll interface{}) {
	k := reflect.TypeOf(coll).Kind()
	if k != reflect.Slice && k != reflect.Array {
		panic(errors.Errorf("invalid vocab: %v of type %T is not a slice", coll, coll))
	}
}

var convertToSlice = func(coll interface{}) []interface{} {
	checkIsCollection(coll)
	var slice []interface{}
	val := reflect.ValueOf(coll)
	for i := 0; i < val.Len(); i++ {
		slice = append(slice, val.Index(i).Interface())
	}
	return slice
}

// UpdateVocabulary is defined on Validator.
func (v *validator) UpdateVocabulary(attributeName string, allowedValues interface{}) {
	attributeName = resolveAlias(attributeName)
	// If this attribute is not registered, delegate to RegisterVocabulary()
	currentValues, ok := v.vocab[attributeName]
	if !ok {
		v.RegisterVocabulary(attributeName, allowedValues)
	}

	unique := map[interface{}]bool{}
	writeUnique := func(all []interface{}) {
		for _, one := range all {
			unique[one] = true
		}
	}

	// merge existing values with new, ensuring uniqueness
	writeUnique(currentValues)
	newValues := convertToSlice(allowedValues)
	writeUnique(newValues)

	v.updateVocabularyFromMap(attributeName, unique)
}

func (v *validator) updateVocabularyFromMap(attributeName string, valuesMap map[interface{}]bool) {
	attributeName = resolveAlias(attributeName)
	var merged []interface{}
	for one := range valuesMap {
		// TODO (anastasiamac) Because it's coming from the map, the order maybe affected
		// and can be unreliable. Not sure how to fix it yet...
		// How can we guarantee the order here?
		merged = append(merged, one)
	}
	v.RegisterVocabulary(attributeName, merged)
}

// checkConflicts returns an error if the constraints Value contains conflicting attributes.
func (v *validator) checkConflicts(cons Value) error {
	attrValues := cons.attributesWithValues()
	attrSet := make(set.Strings)
	for attrTag := range attrValues {
		attrSet.Add(attrTag)
	}
	for _, attrTag := range attrSet.SortedValues() {
		conflicts, ok := v.conflicts[attrTag]
		if !ok {
			continue
		}
		for _, conflict := range conflicts.SortedValues() {
			if !attrSet.Contains(conflict) {
				continue
			}
			id := conflictResolverId(attrTag, conflict)
			if resolver, ok := v.conflictResolvers[id]; ok {
				err := resolver(attrValues)
				if err != nil {
					return errors.Errorf("ambiguous constraints: %q overlaps with %q: %w", attrTag, conflict, err)
				}
				continue
			}
			return errors.Errorf("ambiguous constraints: %q overlaps with %q", attrTag, conflict)
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

// InvalidVocabValueError represents an error that occurs
// when a validation constraint is violated.
// It provides details for the
// invalid inputs, closest valid values and all possible valid values.
type InvalidVocabValueError struct {
	closestValidValues  []string
	allValidValues      []string
	inputAttributeName  string
	inputAttributeValue any
}

func (ve *InvalidVocabValueError) Error() string {
	closestValuesStr := strings.Join(ve.closestValidValues, " ")
	errStr := fmt.Sprintf("invalid constraint value: %v=%v\nvalid values are: %v",
		ve.inputAttributeName, ve.inputAttributeValue, closestValuesStr)

	// Add additional items if there are more than the validValuesCountLimit
	additionalItemsCount := len(ve.allValidValues) - len(ve.closestValidValues)
	if additionalItemsCount > 0 {
		return fmt.Sprintf("%s ...(plus %d more)", errStr, additionalItemsCount)
	}

	return errStr
}

// checkInVocab returns an error if the attribute value is not allowed by the
// vocab which may have been registered for it.
func (v *validator) checkInVocab(attributeName string, attributeValue interface{}) error {
	validValues, ok := v.vocab[resolveAlias(attributeName)]
	if !ok {
		return nil
	}

	validValuesStrLst := make([]string, 0, len(validValues))
	coercedAttributeValue := coerce(attributeValue)
	for _, validValue := range validValues {
		if coerce(validValue) == coercedAttributeValue {
			return nil
		}
		valueStr := fmt.Sprint(validValue)
		validValuesStrLst = append(validValuesStrLst, valueStr)
	}

	// Collect unique constraint values
	validValuesUniqueStrLst := set.NewStrings(validValuesStrLst...).Values()

	// Sort constraint values according to LevenshteinDistance between attributeValue and itself
	if attributeValueStr, ok := attributeValue.(string); ok {
		sort.Slice(validValuesUniqueStrLst, func(i, j int) bool {
			d1 := stringcompare.LevenshteinDistance(validValuesUniqueStrLst[i], attributeValueStr)
			d2 := stringcompare.LevenshteinDistance(validValuesUniqueStrLst[j], attributeValueStr)
			if d1 == d2 {
				return validValuesUniqueStrLst[i] < validValuesUniqueStrLst[j]
			}
			return d1 < d2
		})
	}

	// Find min of len(validValuesUniqueStrLst) and validValuesCountLimit
	min := min(len(validValuesUniqueStrLst), v.validValuesCountLimit)
	closestValidValues := validValuesUniqueStrLst

	// Ensure min is non-negative
	if min > 0 {
		closestValidValues = validValuesUniqueStrLst[:min]
	}

	return &InvalidVocabValueError{
		closestValidValues:  closestValidValues,
		allValidValues:      validValuesUniqueStrLst,
		inputAttributeName:  attributeName,
		inputAttributeValue: attributeValue,
	}
}

// coerce returns v in a format that allows constraint values to be easily
// compared. Its main purpose is to cast all numeric values to float64 (since
// the numbers we compare are generated from json serialization).
func coerce(v interface{}) interface{} {
	switch val := v.(type) {
	case string:
		return v
	// Yes, these are all the same, however we can't put them into a single
	// case, or the value becomes interface{}, which can't be converted to a
	// float64.
	case int:
		return float64(val)
	case int8:
		return float64(val)
	case int16:
		return float64(val)
	case int32:
		return float64(val)
	case int64:
		return float64(val)
	case uint:
		return float64(val)
	case uint8:
		return float64(val)
	case uint16:
		return float64(val)
	case uint32:
		return float64(val)
	case uint64:
		return float64(val)
	case float32:
		return float64(val)
	case float64:
		return val
	}
	return v
}

// withFallbacks returns a copy of v with nil values taken from vFallback.
func withFallbacks(v Value, vFallback Value) Value {
	vAttr := v.attributesWithValues()
	fbAttr := vFallback.attributesWithValues()
	for k, v := range fbAttr {
		if _, ok := vAttr[k]; !ok {
			vAttr[k] = v
		}
	}
	return fromAttributes(vAttr)
}

// Validate is defined on Validator.
//
// The following error types can be expected to be returned:
// - [constraints.InvalidVocabValueError]: When the provided constraint value does not exist in vocabs
func (v *validator) Validate(cons Value) ([]string, error) {
	unsupported := v.checkUnsupported(cons)
	if err := v.checkValidValues(cons); err != nil {
		return unsupported, err
	}
	// Conflicts are validated after values because normally conflicting
	// constraints may be valid based on those constraint values.
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
	attrValues := cons.attributesWithValues()
	var fallbackConflicts []string
	for attrTag := range attrValues {
		fallbackConflicts = append(fallbackConflicts, v.conflicts[attrTag].Values()...)
	}
	// Null out the conflicting consFallback attribute values because
	// cons takes priority. We can't error here because we
	// know that aConflicts contains valid attr names.
	consFallbackMinusConflicts := consFallback.without(fallbackConflicts...)
	// The result is cons with fallbacks coming from any
	// non conflicting consFallback attributes.
	return withFallbacks(cons, consFallbackMinusConflicts), nil
}
