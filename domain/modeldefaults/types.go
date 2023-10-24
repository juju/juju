// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modeldefaults

import (
	"reflect"

	"github.com/juju/juju/environs/config"
)

// DefaultAttributeValue represents a model config default attribute value and
// the hierarchical nature of where defaults can come from within Juju.
type DefaultAttributeValue struct {
	// Controller represents a value that comes from the controller,
	// specifically the cloud.
	Controller any

	// Default represents a value that comes from Juju.
	Default any

	// Region represents a value that come from a model's set cloud region.
	Region any
}

// Defaults represents a set of default values for a given attribute broken down
// based on the different sources of the value.
type Defaults map[string]DefaultAttributeValue

// Has reports if the current Value() of this default attribute is equal to the
// value passed in and also the source of the value. If the current Value() or
// val is nil then false and empty string is returned. If this default attribute
// does not have val then false and empty string is returned.
func (d DefaultAttributeValue) Has(val any) (bool, string) {
	setVal := d.Value()
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
		return true, d.ValueSource()
	}
	return false, ""
}

// Value returns the attribute value that should be used for the default based
// on precedence. If no suitable value can be found then nil will be returned.
func (d DefaultAttributeValue) Value() any {
	if d.Region != nil {
		return d.Region
	}
	if d.Controller != nil {
		return d.Controller
	}
	if d.Default != nil {
		return d.Default
	}
	return nil
}

// ValueSource returns source identifier for the value. If no suitable value can
// be found then empty string will be returned.
func (d DefaultAttributeValue) ValueSource() string {
	if d.Region != nil {
		return config.JujuRegionSource
	}
	if d.Controller != nil {
		return config.JujuControllerSource
	}
	if d.Default != nil {
		return config.JujuDefaultSource
	}
	return ""
}
