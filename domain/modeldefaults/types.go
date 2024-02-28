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
// Because model defaults and the respective sources which defaults come from
// all have their own opinions on how the default will get applied.
// DefaultAttributeValue provides the mechanism for sources to place their
// opinions in one place and for the consuming side (model config) to use the
// default sources opinion.
type DefaultAttributeValue struct {
	// Source describes the source of the default value.
	Source string

	// Strategy is the ApplyStrategy that should be used when deciding how to
	// integrate this default value. If Strategy is the zero value then consult
	// [DefaultAttributeValue.ApplyStrategy] for expected behaviour.
	Strategy ApplyStrategy

	// Value is the default value.
	Value any
}

// Defaults represents a set of default values for a given attribute. Defaults
// should be used to describe the full set of defaults that a model should
// consider for its config.
type Defaults map[string]DefaultAttributeValue

// PreferDefaultApplyStrategy is an [ApplyStrategy] implementation that will
// always the value set in the model default value. If the value for the model
// default is nil then the model config set value will be chosen.
type PreferDefaultApplyStrategy struct{}

// PreferSetApplyStrategy is an [ApplyStrategy] implementation that will always
// prefer the value set in model config before the value being offered by the
// model default. If the set value for model config is nil then the default
// value will be returned. If both values are nil then nil will be returned.
//
// The zero value of this type is safe to use as an [ApplyStrategy].
type PreferSetApplyStrategy struct{}

// ApplyStrategy runs the ApplyStrategy attached to this default value. The
// returned value is the result of what the ApplyStrategy has deemed is the
// value that should be set on the model config. If this DefaultAttributeValue
// has no ApplyStrategy set then by default we pass the decision to
// [PreferSetApplyStrategy].
func (d DefaultAttributeValue) ApplyStrategy(setVal any) any {
	if d.Strategy == nil {
		strategy := PreferSetApplyStrategy{}
		return strategy.Apply(d.Value, setVal)
	}
	return d.Strategy.Apply(d.Value, setVal)
}

// Has reports if the current [DefaultAttributeValue.Value] is equal to the
// value passed in. The source of the default value is also returned when the
// values are equal. If the current value of [DefaultAttributeValue.Value] or
// val is nil then false and empty string for source is returned.
//
// For legacy reasons we have worked with types of "any" for model config values
// and trying to apply comparison logic over these types is hard to get right.
// For this reason this function only considers values to be equal if their
// types are comparable via == and or the type of Value is of []any, in which
// case we will defer to the reflect package for DeepEqual.
//
// This is carry over logic from legacy Juju. Over time we can look at removing
// the use of any for more concrete types.
func (d DefaultAttributeValue) Has(val any) (bool, string) {
	setVal := d.Value
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

// Apply implements [ApplyStrategy] interface for [PreferDefaultApplyStrategy]
func (*PreferDefaultApplyStrategy) Apply(defaultVal, setVal any) any {
	if defaultVal == nil {
		return setVal
	}
	return defaultVal
}

// Apply implements [ApplyStrategy] interface for [PreferSetApplyStrategy].
func (*PreferSetApplyStrategy) Apply(defaultVal, setVal any) any {
	if setVal != nil {
		return setVal
	}
	return defaultVal
}
