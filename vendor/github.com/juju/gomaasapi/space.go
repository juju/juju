// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package gomaasapi

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
	"github.com/juju/version"
)

type space struct {
	// Add the controller in when we need to do things with the space.
	// controller Controller

	resourceURI string

	id   int
	name string

	subnets []*subnet
}

// Id implements Space.
func (s *space) ID() int {
	return s.id
}

// Name implements Space.
func (s *space) Name() string {
	return s.name
}

// Subnets implements Space.
func (s *space) Subnets() []Subnet {
	var result []Subnet
	for _, subnet := range s.subnets {
		result = append(result, subnet)
	}
	return result
}

func readSpaces(controllerVersion version.Number, source interface{}) ([]*space, error) {
	checker := schema.List(schema.StringMap(schema.Any()))
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "space base schema check failed")
	}
	valid := coerced.([]interface{})

	var deserialisationVersion version.Number
	for v := range spaceDeserializationFuncs {
		if v.Compare(deserialisationVersion) > 0 && v.Compare(controllerVersion) <= 0 {
			deserialisationVersion = v
		}
	}
	if deserialisationVersion == version.Zero {
		return nil, errors.Errorf("no space read func for version %s", controllerVersion)
	}
	readFunc := spaceDeserializationFuncs[deserialisationVersion]
	return readSpaceList(valid, readFunc)
}

// readSpaceList expects the values of the sourceList to be string maps.
func readSpaceList(sourceList []interface{}, readFunc spaceDeserializationFunc) ([]*space, error) {
	result := make([]*space, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for space %d, %T", i, value)
		}
		space, err := readFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "space %d", i)
		}
		result = append(result, space)
	}
	return result, nil
}

type spaceDeserializationFunc func(map[string]interface{}) (*space, error)

var spaceDeserializationFuncs = map[version.Number]spaceDeserializationFunc{
	twoDotOh: space_2_0,
}

func space_2_0(source map[string]interface{}) (*space, error) {
	fields := schema.Fields{
		"resource_uri": schema.String(),
		"id":           schema.ForceInt(),
		"name":         schema.String(),
		"subnets":      schema.List(schema.StringMap(schema.Any())),
	}
	checker := schema.FieldMap(fields, nil) // no defaults
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "space 2.0 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.

	subnets, err := readSubnetList(valid["subnets"].([]interface{}), subnet_2_0)
	if err != nil {
		return nil, errors.Trace(err)
	}

	result := &space{
		resourceURI: valid["resource_uri"].(string),
		id:          valid["id"].(int),
		name:        valid["name"].(string),
		subnets:     subnets,
	}
	return result, nil
}
