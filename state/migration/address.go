// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
)

type AddressArgs struct {
	Value       string
	Type        string
	NetworkName string
	Scope       string
	Origin      string
}

func newAddress(args AddressArgs) *address {
	return &address{
		Version:      1,
		Value_:       args.Value,
		Type_:        args.Type,
		NetworkName_: args.NetworkName,
		Scope_:       args.Scope,
		Origin_:      args.Origin,
	}
}

// address represents an IP Address of some form.
type address struct {
	Version int `yaml:"version"`

	Value_       string `yaml:"value"`
	Type_        string `yaml:"type"`
	NetworkName_ string `yaml:"network-name,omitempty"`
	Scope_       string `yaml:"scope,omitempty"`
	Origin_      string `yaml:"origin,omitempty"`
}

// Value implements Address.Value interface method.
func (a *address) Value() string {
	return a.Value_
}

// Type implements Address.Type interface method.
func (a *address) Type() string {
	return a.Type_
}

// NetworkName implements Address.NetworkName interface method.
func (a *address) NetworkName() string {
	return a.NetworkName_
}

// Scope implements Address.Scope interface method.
func (a *address) Scope() string {
	return a.Scope_
}

// Origin implements Address.Origin interface method.
func (a *address) Origin() string {
	return a.Origin_
}

// importAddress constructs a new Address from a map that in normal usage situations
// will be the result of interpreting a large YAML document.
//
// This method is a package internal serialisation method.
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
		"value":        schema.String(),
		"type":         schema.String(),
		"network-name": schema.String(),
		"scope":        schema.String(),
		"origin":       schema.String(),
	}
	// Some values don't have to be there.
	defaults := schema.Defaults{
		"network-name": "",
		"scope":        "",
		"origin":       "",
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
		Version:      1,
		Value_:       valid["value"].(string),
		Type_:        valid["type"].(string),
		NetworkName_: valid["network-name"].(string),
		Scope_:       valid["scope"].(string),
		Origin_:      valid["origin"].(string),
	}, nil
}
