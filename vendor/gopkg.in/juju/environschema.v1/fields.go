// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

// Package environschema implements a way to specify
// configuration attributes for Juju environments.
package environschema // import "gopkg.in/juju/environschema.v1"

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/schema"
	"github.com/juju/utils/keyvalues"
)

// What to do about reading content from paths?
// Could just have a load of client-side special cases.

// Fields holds a map from attribute name to
// information about that attribute.
type Fields map[string]Attr

type Attr struct {
	// Description holds a human-readable description
	// of the attribute.
	Description string `json:"description"`

	// Type holds the type of the attribute value.
	Type FieldType `json:"type"`

	// Group holds the group that the attribute belongs to.
	// All attributes within a Fields that have the same Group
	// attribute are considered to be part of the same group.
	Group Group `json:"group"`

	// Immutable specifies whether the attribute cannot
	// be changed once set.
	Immutable bool

	// Mandatory specifies whether the attribute
	// must be provided.
	Mandatory bool `json:"mandatory,omitempty"`

	// Secret specifies whether the attribute should be
	// considered secret.
	Secret bool `json:"is-secret,omitempty"`

	// EnvVar holds the environment variable
	// that will be used to obtain the default value
	// if it isn't specified.
	EnvVar string `json:"env-var,omitempty"`

	// EnvVars holds additional environment
	// variables to be used if the value in EnvVar is
	// not available, from highest to lowest priority.
	EnvVars []string `json:"env-vars,omitempty"`

	// Example holds an example value for the attribute
	// that can be used to produce a plausible-looking
	// entry for the attribute without necessarily using
	// it as a default value.
	//
	// TODO if the example holds some special values, use
	// it as a template to generate initial random values
	// (for example for admin-password) ?
	Example interface{} `json:"example,omitempty"`

	// Values holds the set of all possible values of the attribute.
	Values []interface{} `json:"values,omitempty"`
}

// Checker returns a checker that can be used to coerce values into the
// type of the attribute. Specifically, string is always supported for
// any checker type.
func (attr Attr) Checker() (schema.Checker, error) {
	checker := checkers[attr.Type]
	if checker == nil {
		return nil, fmt.Errorf("invalid type %q", attr.Type)
	}
	if len(attr.Values) == 0 {
		return checker, nil
	}
	return oneOfValues(checker, attr.Values)
}

// Group describes the grouping of attributes.
type Group string

// The following constants are the initially defined group values.
const (
	// JujuGroup groups attributes defined by Juju that may
	// not be specified by a user.
	JujuGroup Group = "juju"

	// EnvironGroup groups attributes that are defined across all
	// possible Juju environments.
	EnvironGroup Group = "environ"

	// AccountGroup groups attributes that define a user account
	// used by a provider.
	AccountGroup Group = "account"

	// ProviderGroup groups attributes defined by the provider
	// that are not account credentials. This is also the default
	// group.
	ProviderGroup Group = ""
)

// FieldType describes the type of an attribute value.
type FieldType string

// The following constants are the possible type values.
// The "canonical Go type" is the type that the will be
// the result of a successful Coerce call.
const (
	// Tstring represents a string type. Its canonical Go type is string.
	Tstring FieldType = "string"

	// Tbool represents a boolean type. Its canonical Go type is bool.
	Tbool FieldType = "bool"

	// Tint represents an integer type. Its canonical Go type is int.
	Tint FieldType = "int"

	// Tattrs represents an attribute map. Its canonical Go type is map[string]string.
	Tattrs FieldType = "attrs"
)

var checkers = map[FieldType]schema.Checker{
	Tstring: schema.String(),
	Tbool:   schema.Bool(),
	Tint:    schema.ForceInt(),
	Tattrs:  attrsChecker{},
}

// Alternative possibilities to ValidationSchema to bear in mind for
// the future:
// func (s Fields) Checker() schema.Checker
// func (s Fields) Validate(value map[string]interface{}) (v map[string] interface{}, extra []string, err error)

// ValidationSchema returns values suitable for passing to
// schema.FieldMap to create a schema.Checker that will validate the given fields.
// It will return an error if the fields are invalid.
//
// The Defaults return value will contain entries for all non-mandatory
// attributes set to schema.Omit. It is the responsibility of the
// client to set any actual default values as required.
func (s Fields) ValidationSchema() (schema.Fields, schema.Defaults, error) {
	fields := make(schema.Fields)
	defaults := make(schema.Defaults)
	for name, attr := range s {
		path := []string{name}
		checker, err := attr.Checker()
		if err != nil {
			return nil, nil, errors.Annotatef(err, "%s", mkPath(path))
		}
		if !attr.Mandatory {
			defaults[name] = schema.Omit
		}
		fields[name] = checker
	}
	return fields, defaults, nil
}

// oneOfValues returns a checker that coerces its value
// using the supplied checker, then checks that the
// resulting value is equal to one of the given values.
func oneOfValues(checker schema.Checker, values []interface{}) (schema.Checker, error) {
	cvalues := make([]interface{}, len(values))
	for i, v := range values {
		cv, err := checker.Coerce(v, nil)
		if err != nil {
			return nil, fmt.Errorf("invalid enumerated value: %v", err)
		}
		cvalues[i] = cv
	}
	return oneOfValuesChecker{
		vals:    cvalues,
		checker: checker,
	}, nil
}

type oneOfValuesChecker struct {
	vals    []interface{}
	checker schema.Checker
}

// Coerce implements schema.Checker.Coerce.
func (c oneOfValuesChecker) Coerce(v interface{}, path []string) (interface{}, error) {
	v, err := c.checker.Coerce(v, path)
	if err != nil {
		return v, err
	}
	for _, allow := range c.vals {
		if allow == v {
			return v, nil
		}
	}
	return nil, fmt.Errorf("%sexpected one of %v, got %#v", pathPrefix(path), c.vals, v)
}

type attrsChecker struct{}

var (
	attrMapChecker   = schema.Map(schema.String(), schema.String())
	attrSliceChecker = schema.List(schema.String())
)

func (c attrsChecker) Coerce(v interface{}, path []string) (interface{}, error) {
	// TODO consider allowing only the map variant.
	switch reflect.TypeOf(v).Kind() {
	case reflect.String:
		s, err := schema.String().Coerce(v, path)
		if err != nil {
			return nil, errors.Mask(err)
		}
		result, err := keyvalues.Parse(strings.Fields(s.(string)), true)
		if err != nil {
			return nil, fmt.Errorf("%s%v", pathPrefix(path), err)
		}
		return result, nil
	case reflect.Slice:
		slice0, err := attrSliceChecker.Coerce(v, path)
		if err != nil {
			return nil, errors.Mask(err)
		}
		slice := slice0.([]interface{})
		fields := make([]string, len(slice))
		for i, f := range slice {
			fields[i] = f.(string)
		}
		result, err := keyvalues.Parse(fields, true)
		if err != nil {
			return nil, fmt.Errorf("%s%v", pathPrefix(path), err)
		}
		return result, nil
	case reflect.Map:
		imap0, err := attrMapChecker.Coerce(v, path)
		if err != nil {
			return nil, errors.Mask(err)
		}
		imap := imap0.(map[interface{}]interface{})
		result := make(map[string]string)
		for k, v := range imap {
			result[k.(string)] = v.(string)
		}
		return result, nil
	default:
		return nil, errors.Errorf("%sunexpected type for value, got %T(%v)", pathPrefix(path), v, v)
	}
}

// pathPrefix returns an error message prefix holding
// the concatenation of the path elements. If path
// starts with a ".", the dot is omitted.
func pathPrefix(path []string) string {
	if p := mkPath(path); p != "" {
		return p + ": "
	}
	return ""
}

// mkPath returns a string holding
// the concatenation of the path elements.
// If path starts with a ".", the dot is omitted.
func mkPath(path []string) string {
	if len(path) == 0 {
		return ""
	}
	if path[0] == "." {
		return strings.Join(path[1:], "")
	}
	return strings.Join(path, "")
}

// ExampleYAML returns the fields formatted as a YAML
// example, with non-mandatory fields commented out,
// like the providers do currently.
func (s Fields) ExampleYAML() []byte {
	panic("unimplemented")
}
