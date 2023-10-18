// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modeldefaults

// DefaultAttributeValue represents a model config default attribute value and
// the hierarchical nature of where defaults can come from within Juju.
type DefaultAttributeValue struct {
	// Controller represents a value that come from the controller
	Controller any

	// Default represents a value that comes from Juju.
	Default any

	// Region represents a value that come from a model's set cloud region.
	Region any
}

type Defaults map[string]DefaultAttributeValue

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
