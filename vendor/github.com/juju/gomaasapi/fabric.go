// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package gomaasapi

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
	"github.com/juju/version"
)

type fabric struct {
	// Add the controller in when we need to do things with the fabric.
	// controller Controller

	resourceURI string

	id        int
	name      string
	classType string

	vlans []*vlan
}

// ID implements Fabric.
func (f *fabric) ID() int {
	return f.id
}

// Name implements Fabric.
func (f *fabric) Name() string {
	return f.name
}

// ClassType implements Fabric.
func (f *fabric) ClassType() string {
	return f.classType
}

// VLANs implements Fabric.
func (f *fabric) VLANs() []VLAN {
	var result []VLAN
	for _, v := range f.vlans {
		result = append(result, v)
	}
	return result
}

func readFabrics(controllerVersion version.Number, source interface{}) ([]*fabric, error) {
	checker := schema.List(schema.StringMap(schema.Any()))
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "fabric base schema check failed")
	}
	valid := coerced.([]interface{})

	var deserialisationVersion version.Number
	for v := range fabricDeserializationFuncs {
		if v.Compare(deserialisationVersion) > 0 && v.Compare(controllerVersion) <= 0 {
			deserialisationVersion = v
		}
	}
	if deserialisationVersion == version.Zero {
		return nil, errors.Errorf("no fabric read func for version %s", controllerVersion)
	}
	readFunc := fabricDeserializationFuncs[deserialisationVersion]
	return readFabricList(valid, readFunc)
}

// readFabricList expects the values of the sourceList to be string maps.
func readFabricList(sourceList []interface{}, readFunc fabricDeserializationFunc) ([]*fabric, error) {
	result := make([]*fabric, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for fabric %d, %T", i, value)
		}
		fabric, err := readFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "fabric %d", i)
		}
		result = append(result, fabric)
	}
	return result, nil
}

type fabricDeserializationFunc func(map[string]interface{}) (*fabric, error)

var fabricDeserializationFuncs = map[version.Number]fabricDeserializationFunc{
	twoDotOh: fabric_2_0,
}

func fabric_2_0(source map[string]interface{}) (*fabric, error) {
	fields := schema.Fields{
		"resource_uri": schema.String(),
		"id":           schema.ForceInt(),
		"name":         schema.String(),
		"class_type":   schema.OneOf(schema.Nil(""), schema.String()),
		"vlans":        schema.List(schema.StringMap(schema.Any())),
	}
	checker := schema.FieldMap(fields, nil) // no defaults
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "fabric 2.0 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.

	vlans, err := readVLANList(valid["vlans"].([]interface{}), vlan_2_0)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Since the class_type is optional, we use the two part cast assignment. If
	// the cast fails, then we get the default value we care about, which is the
	// empty string.
	classType, _ := valid["class_type"].(string)

	result := &fabric{
		resourceURI: valid["resource_uri"].(string),
		id:          valid["id"].(int),
		name:        valid["name"].(string),
		classType:   classType,
		vlans:       vlans,
	}
	return result, nil
}
