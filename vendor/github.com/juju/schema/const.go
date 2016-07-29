// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package schema

import (
	"fmt"
	"reflect"
)

// Const returns a Checker that only succeeds if the input matches
// value exactly.  The value is compared with reflect.DeepEqual.
func Const(value interface{}) Checker {
	return constC{value}
}

type constC struct {
	value interface{}
}

func (c constC) Coerce(v interface{}, path []string) (interface{}, error) {
	if reflect.DeepEqual(v, c.value) {
		return v, nil
	}
	return nil, error_{fmt.Sprintf("%#v", c.value), v, path}
}

// Nil returns a Checker that only succeeds if the input is nil. To tweak the
// error message, valueLabel can contain a label of the value being checked to
// be empty, e.g. "my special name". If valueLabel is "", "value" will be used
// as a label instead.
//
// Example 1:
// schema.Nil("widget").Coerce(42, nil) will return an error message
// like `expected empty widget, got int(42)`.
//
// Example 2:
// schema.Nil("").Coerce("", nil) will return an error message like
// `expected empty value, got string("")`.
func Nil(valueLabel string) Checker {
	if valueLabel == "" {
		valueLabel = "value"
	}
	return nilC{valueLabel}
}

type nilC struct {
	valueLabel string
}

func (c nilC) Coerce(v interface{}, path []string) (interface{}, error) {
	if reflect.DeepEqual(v, nil) {
		return v, nil
	}
	label := fmt.Sprintf("empty %s", c.valueLabel)
	return nil, error_{label, v, path}
}
