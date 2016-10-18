// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
)

// AddressArgs is an argument struct used to create a new internal address
// type that supports the Address interface.
type AddressArgs struct {
	Value  string
	Type   string
	Scope  string
	Origin string
}

func newAddress(args AddressArgs) *address {
	return &address{
		Version: 1,
		Value_:  args.Value,
		Type_:   args.Type,
		Scope_:  args.Scope,
		Origin_: args.Origin,
	}
}

// address represents an IP Address of some form.
type address struct {
	Version int `yaml:"version"`

	Value_  string `yaml:"value"`
	Type_   string `yaml:"type"`
	Scope_  string `yaml:"scope,omitempty"`
	Origin_ string `yaml:"origin,omitempty"`
}

// Value implements Address.
func (a *address) Value() string {
	return a.Value_
}

// Type implements Address.
func (a *address) Type() string {
	return a.Type_
}

// Scope implements Address.
func (a *address) Scope() string {
	return a.Scope_
}

// Origin implements Address.
func (a *address) Origin() string {
	return a.Origin_
}

func importAddresses(sourceList []interface{}) ([]*address, error) {
	var result []*address
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for address %d, %T", i, value)
		}
		addr, err := importAddress(source)
		if err != nil {
			return nil, errors.Trace(err)
		}
		result = append(result, addr)
	}
	return result, nil
}

// importAddress constructs a new Address from a map representing a serialised
// Address instance.
func importAddress(source map[string]interface{}) (*address, error) {
	version, err := getVersion(source)
	if err != nil {
		return nil, errors.Annotate(err, "address version schema check failed")
	}

	importFunc, ok := addressDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}

	return importFunc(source)
}

type addressDeserializationFunc func(map[string]interface{}) (*address, error)

var addressDeserializationFuncs = map[int]addressDeserializationFunc{
	1: importAddressV1,
}

func importAddressV1(source map[string]interface{}) (*address, error) {
	fields := schema.Fields{
		"value":  schema.String(),
		"type":   schema.String(),
		"scope":  schema.String(),
		"origin": schema.String(),
	}
	// Some values don't have to be there.
	defaults := schema.Defaults{
		"scope":  "",
		"origin": "",
	}
	checker := schema.FieldMap(fields, defaults)

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "address v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.

	return &address{
		Version: 1,
		Value_:  valid["value"].(string),
		Type_:   valid["type"].(string),
		Scope_:  valid["scope"].(string),
		Origin_: valid["origin"].(string),
	}, nil
}
