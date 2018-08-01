//go:generate go run internal/cmd/gentest/gentest.go schema.json generated_validator_test.go
//go:generate go run internal/cmd/genmaybe/genmaybe.go

// Package jsval implements an input validator, based on JSON Schema.
// The main purpose is to validate JSON Schemas (see
// https://github.com/lestrrat/go-jsschema), and to automatically
// generate validators from schemas, but jsval can be used independently
// of JSON Schema.
package jsval

import "github.com/pkg/errors"

// New creates a new JSVal instance.
func New() *JSVal {
	return &JSVal{
		ConstraintMap: &ConstraintMap{},
	}
}

// Validate validates the input, and return an error
// if any of the validations fail
func (v *JSVal) Validate(x interface{}) error {
	name := v.Name
	if len(name) == 0 {
		return errors.Wrapf(v.root.Validate(x), "validator %p failed", v)
	}
	return errors.Wrapf(v.root.Validate(x), "validator %s failed", name)
}

// SetName sets the name for the validator
func (v *JSVal) SetName(s string) *JSVal {
	v.Name = s
	return v
}

// SetRoot sets the root Constraint object.
func (v *JSVal) SetRoot(c Constraint) *JSVal {
	v.root = c
	return v
}

// Root returns the root Constraint object.
func (v *JSVal) Root() Constraint {
	return v.root
}

// SetConstraintMap allows you to set the map that is referred to
// when resolving JSON references within constraints. By setting
// this to a common map, for example, you can share the same references
// to save you some memory space and sanity. See an example in the
// `generated_validator_test.go` file.
func (v *JSVal) SetConstraintMap(cm *ConstraintMap) *JSVal {
	v.ConstraintMap = cm
	return v
}

func (p JSValSlice) Len() int           { return len(p) }
func (p JSValSlice) Less(i, j int) bool { return p[i].Name < p[j].Name }
func (p JSValSlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
