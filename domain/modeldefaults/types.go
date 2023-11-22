// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modeldefaults

import (
	"reflect"
)

// ApplyStrategy describes a strategy for how default values should be
// applied to model config.
type ApplyStrategy interface {
	// Apply evaluates both the model config default value and that of the
	// already set value on model config and returns the resultant value that
	// should be set on model config.
	Apply(defaultVal, setVal any) any
}

// DefaultAttributeValue represents a model config default attribute value and
// the hierarchical nature of where defaults can come from within Juju.
//
// Because model defaults and the respective sources from which defaults come
// from all have their own opinions on how the default will get applied to model
// config and the ultimate say in what the value is DefaultAttributeValue
// provides the mechanism for sources to place their opinions in one place and
// for the consuming side (model config) to have a one size fits all approach
// for taking on the defaults.
type DefaultAttributeValue struct {
	// Source describes the source of the default value.
	Source string

	// Strategy is the ApplyStrategy that should be used when deciding how to
	// integrate this default value.
	Strategy ApplyStrategy

	// V is the default value.
	V any
}

// Defaults represents a set of default values for a given attribute. Defaults
// should be used to describe the full set of defaults that a model should
// consider for it's config.
type Defaults map[string]DefaultAttributeValue

// ApplyStrategy runs the ApplyStrategy attached to this default value. The
// returned value is the result of what the ApplyStrategy has deemed is the
// value that should be set on the model config. If this DefaultAttributeValue
// has no ApplyStrategy set then the setVal passed to this function is returned.
func (d DefaultAttributeValue) ApplyStrategy(setVal any) any {
	if d.Strategy == nil {
		return setVal
	}
	return d.Strategy.Apply(d.V, setVal)
}

// Has reports if the current V of this default attribute is equal to the
// value passed in. The source of the default value is also returned when the
// values are equal. If the current value of V or val is nil then false and
// empty string is returned.
//
// For legacy reasons in Juju's life we have have worked with types of "any" for
// model config values and trying to apply comparison logic over these types is
// hard to get right. For this purpose this function only considers values to be
// equal if their types are comparable via == and or the type of V is []any then
// we will defer to the reflect package for DeepEqual.
//
// This is carry over logic from legacy Juju. Over time we can look at removing
// the use of any for more concrete types.
func (d DefaultAttributeValue) Has(val any) (bool, string) {
	setVal := d.V
	if setVal == nil || val == nil {
		return false, ""
	}

	equal := false
	switch setVal.(type) {
	case []any:
		equal = reflect.DeepEqual(setVal, val)
	default:
		equal = setVal == val
	}
	if equal {
		return true, d.Source
	}
	return false, ""
}
