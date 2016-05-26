// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
)

type ipaddresss struct {
	Version     int          `yaml:"version"`
	IPAddresss_ []*ipaddress `yaml:"ipaddresss"`
}

type ipaddress struct {
}

// IPAddressArgs is an argument struct used to create a
// new internal ipaddress type that supports the IPAddress interface.
type IPAddressArgs struct {
}

func newIPAddress(args IPAddressArgs) *ipaddress {
	return &ipaddress{}
}

func importIPAddresss(source map[string]interface{}) ([]*ipaddress, error) {
	checker := versionedChecker("ipaddresss")
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "ipaddresss version schema check failed")
	}
	valid := coerced.(map[string]interface{})

	version := int(valid["version"].(int64))
	importFunc, ok := ipaddressDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}
	sourceList := valid["ipaddresss"].([]interface{})
	return importIPAddressList(sourceList, importFunc)
}

func importIPAddressList(sourceList []interface{}, importFunc ipaddressDeserializationFunc) ([]*ipaddress, error) {
	result := make([]*ipaddress, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for ipaddress %d, %T", i, value)
		}
		ipaddress, err := importFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "ipaddress %d", i)
		}
		result = append(result, ipaddress)
	}
	return result, nil
}

type ipaddressDeserializationFunc func(map[string]interface{}) (*ipaddress, error)

var ipaddressDeserializationFuncs = map[int]ipaddressDeserializationFunc{
	1: importIPAddressV1,
}

func importIPAddressV1(source map[string]interface{}) (*ipaddress, error) {
	fields := schema.Fields{}
	// Some values don't have to be there.
	defaults := schema.Defaults{
		"provider-id": "",
	}
	checker := schema.FieldMap(fields, defaults)

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "ipaddress v1 schema check failed")
	}
	// XXX valid :=
	_ = coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.
	return &ipaddress{}, nil
}
