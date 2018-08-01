// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package gomaasapi

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
	"github.com/juju/version"
)

type link struct {
	id        int
	mode      string
	subnet    *subnet
	ipAddress string
}

// NOTE: not using lowercase L as the receiver as it is a horrible idea.
// Instead using 'k'.

// ID implements Link.
func (k *link) ID() int {
	return k.id
}

// Mode implements Link.
func (k *link) Mode() string {
	return k.mode
}

// Subnet implements Link.
func (k *link) Subnet() Subnet {
	if k.subnet == nil {
		return nil
	}
	return k.subnet
}

// IPAddress implements Link.
func (k *link) IPAddress() string {
	return k.ipAddress
}

func readLinks(controllerVersion version.Number, source interface{}) ([]*link, error) {
	checker := schema.List(schema.StringMap(schema.Any()))
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, WrapWithDeserializationError(err, "link base schema check failed")
	}
	valid := coerced.([]interface{})

	var deserialisationVersion version.Number
	for v := range linkDeserializationFuncs {
		if v.Compare(deserialisationVersion) > 0 && v.Compare(controllerVersion) <= 0 {
			deserialisationVersion = v
		}
	}
	if deserialisationVersion == version.Zero {
		return nil, NewUnsupportedVersionError("no link read func for version %s", controllerVersion)
	}
	readFunc := linkDeserializationFuncs[deserialisationVersion]
	return readLinkList(valid, readFunc)
}

// readLinkList expects the values of the sourceList to be string maps.
func readLinkList(sourceList []interface{}, readFunc linkDeserializationFunc) ([]*link, error) {
	result := make([]*link, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, NewDeserializationError("unexpected value for link %d, %T", i, value)
		}
		link, err := readFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "link %d", i)
		}
		result = append(result, link)
	}
	return result, nil
}

type linkDeserializationFunc func(map[string]interface{}) (*link, error)

var linkDeserializationFuncs = map[version.Number]linkDeserializationFunc{
	twoDotOh: link_2_0,
}

func link_2_0(source map[string]interface{}) (*link, error) {
	fields := schema.Fields{
		"id":         schema.ForceInt(),
		"mode":       schema.String(),
		"subnet":     schema.StringMap(schema.Any()),
		"ip_address": schema.String(),
	}
	defaults := schema.Defaults{
		"ip_address": "",
		"subnet":     schema.Omit,
	}
	checker := schema.FieldMap(fields, defaults)
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, WrapWithDeserializationError(err, "link 2.0 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.

	var subnet *subnet
	if value, ok := valid["subnet"]; ok {
		subnet, err = subnet_2_0(value.(map[string]interface{}))
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	result := &link{
		id:        valid["id"].(int),
		mode:      valid["mode"].(string),
		subnet:    subnet,
		ipAddress: valid["ip_address"].(string),
	}
	return result, nil
}
