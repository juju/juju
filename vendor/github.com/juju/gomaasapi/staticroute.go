// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package gomaasapi

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
	"github.com/juju/version"
)

type staticRoute struct {
	resourceURI string

	id          int
	source      *subnet
	destination *subnet
	gatewayIP   string
	metric      int
}

// Id implements StaticRoute.
func (s *staticRoute) ID() int {
	return s.id
}

// Source implements StaticRoute.
func (s *staticRoute) Source() Subnet {
	return s.source
}

// Destination implements StaticRoute.
func (s *staticRoute) Destination() Subnet {
	return s.destination
}

// GatewayIP implements StaticRoute.
func (s *staticRoute) GatewayIP() string {
	return s.gatewayIP
}

// Metric implements StaticRoute.
func (s *staticRoute) Metric() int {
	return s.metric
}

func readStaticRoutes(controllerVersion version.Number, source interface{}) ([]*staticRoute, error) {
	checker := schema.List(schema.StringMap(schema.Any()))
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "static-route base schema check failed")
	}
	valid := coerced.([]interface{})

	var deserialisationVersion version.Number
	for v := range staticRouteDeserializationFuncs {
		if v.Compare(deserialisationVersion) > 0 && v.Compare(controllerVersion) <= 0 {
			deserialisationVersion = v
		}
	}
	if deserialisationVersion == version.Zero {
		return nil, errors.Errorf("no static-route read func for version %s", controllerVersion)
	}
	readFunc := staticRouteDeserializationFuncs[deserialisationVersion]
	return readStaticRouteList(valid, readFunc)
}

// readStaticRouteList expects the values of the sourceList to be string maps.
func readStaticRouteList(sourceList []interface{}, readFunc staticRouteDeserializationFunc) ([]*staticRoute, error) {
	result := make([]*staticRoute, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for static-route %d, %T", i, value)
		}
		staticRoute, err := readFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "static-route %d", i)
		}
		result = append(result, staticRoute)
	}
	return result, nil
}

type staticRouteDeserializationFunc func(map[string]interface{}) (*staticRoute, error)

var staticRouteDeserializationFuncs = map[version.Number]staticRouteDeserializationFunc{
	twoDotOh: staticRoute_2_0,
}

func staticRoute_2_0(source map[string]interface{}) (*staticRoute, error) {
	fields := schema.Fields{
		"resource_uri": schema.String(),
		"id":           schema.ForceInt(),
		"source":       schema.StringMap(schema.Any()),
		"destination":  schema.StringMap(schema.Any()),
		"gateway_ip":   schema.String(),
		"metric":       schema.ForceInt(),
	}
	checker := schema.FieldMap(fields, nil) // no defaults
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "static-route 2.0 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.

	// readSubnetList takes a list of interfaces. We happen to have 2 subnets
	// to parse, that are in different keys, but we might as well wrap them up
	// together and pass them in.
	subnets, err := readSubnetList([]interface{}{valid["source"], valid["destination"]}, subnet_2_0)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(subnets) != 2 {
		// how could we get here?
		return nil, errors.Errorf("subnets somehow parsed into the wrong number of items (expected 2): %d", len(subnets))
	}

	result := &staticRoute{
		resourceURI: valid["resource_uri"].(string),
		id:          valid["id"].(int),
		gatewayIP:   valid["gateway_ip"].(string),
		metric:      valid["metric"].(int),
		source:      subnets[0],
		destination: subnets[1],
	}
	return result, nil
}
