// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
)

type subnets struct {
	Version  int       `yaml:"version"`
	Subnets_ []*subnet `yaml:"subnets"`
}

type subnet struct {
}

// SubnetArgs is an argument struct used to create a
// new internal subnet type that supports the Subnet interface.
type SubnetArgs struct {
}

func newSubnet(args SubnetArgs) *subnet {
	return &subnet{}
}

func importSubnets(source map[string]interface{}) ([]*subnet, error) {
	checker := versionedChecker("subnets")
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "subnets version schema check failed")
	}
	valid := coerced.(map[string]interface{})

	version := int(valid["version"].(int64))
	importFunc, ok := subnetDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}
	sourceList := valid["subnets"].([]interface{})
	return importSubnetList(sourceList, importFunc)
}

func importSubnetList(sourceList []interface{}, importFunc subnetDeserializationFunc) ([]*subnet, error) {
	result := make([]*subnet, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for subnet %d, %T", i, value)
		}
		subnet, err := importFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "subnet %d", i)
		}
		result = append(result, subnet)
	}
	return result, nil
}

type subnetDeserializationFunc func(map[string]interface{}) (*subnet, error)

var subnetDeserializationFuncs = map[int]subnetDeserializationFunc{
	1: importSubnetV1,
}

func importSubnetV1(source map[string]interface{}) (*subnet, error) {
	fields := schema.Fields{}
	defaults := schema.Defaults{}
	checker := schema.FieldMap(fields, defaults)

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "subnet v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})

	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.
	return &subnet{}, nil
}
