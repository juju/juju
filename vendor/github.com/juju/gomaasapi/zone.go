// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package gomaasapi

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
	"github.com/juju/version"
)

type zone struct {
	// Add the controller in when we need to do things with the zone.
	// controller Controller

	resourceURI string

	name        string
	description string
}

// Name implements Zone.
func (z *zone) Name() string {
	return z.name
}

// Description implements Zone.
func (z *zone) Description() string {
	return z.description
}

func readZones(controllerVersion version.Number, source interface{}) ([]*zone, error) {
	checker := schema.List(schema.StringMap(schema.Any()))
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "zone base schema check failed")
	}
	valid := coerced.([]interface{})

	var deserialisationVersion version.Number
	for v := range zoneDeserializationFuncs {
		if v.Compare(deserialisationVersion) > 0 && v.Compare(controllerVersion) <= 0 {
			deserialisationVersion = v
		}
	}
	if deserialisationVersion == version.Zero {
		return nil, errors.Errorf("no zone read func for version %s", controllerVersion)
	}
	readFunc := zoneDeserializationFuncs[deserialisationVersion]
	return readZoneList(valid, readFunc)
}

// readZoneList expects the values of the sourceList to be string maps.
func readZoneList(sourceList []interface{}, readFunc zoneDeserializationFunc) ([]*zone, error) {
	result := make([]*zone, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for zone %d, %T", i, value)
		}
		zone, err := readFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "zone %d", i)
		}
		result = append(result, zone)
	}
	return result, nil
}

type zoneDeserializationFunc func(map[string]interface{}) (*zone, error)

var zoneDeserializationFuncs = map[version.Number]zoneDeserializationFunc{
	twoDotOh: zone_2_0,
}

func zone_2_0(source map[string]interface{}) (*zone, error) {
	fields := schema.Fields{
		"name":         schema.String(),
		"description":  schema.String(),
		"resource_uri": schema.String(),
	}
	checker := schema.FieldMap(fields, nil) // no defaults
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "zone 2.0 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.

	result := &zone{
		name:        valid["name"].(string),
		description: valid["description"].(string),
		resourceURI: valid["resource_uri"].(string),
	}
	return result, nil
}
