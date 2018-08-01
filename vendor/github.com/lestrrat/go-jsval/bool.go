package jsval

import (
	"errors"
	"reflect"
)

// Boolean creates a new BooleanConsraint
func Boolean() *BooleanConstraint {
	return &BooleanConstraint{}
}

// Default specifies the default value to apply
func (bc *BooleanConstraint) Default(v interface{}) *BooleanConstraint {
	bc.defaultValue.initialized = true
	bc.defaultValue.value = v
	return bc
}

// Validate vaidates the value against the given value
func (bc *BooleanConstraint) Validate(v interface{}) error {
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Bool:
	default:
		return errors.New("value is not a boolean")
	}
	return nil
}
