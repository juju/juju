// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package devices

import (
	"strconv"
	"strings"

	"github.com/juju/juju/internal/errors"
)

var deviceParseErr = errors.Errorf("cannot parse device constraints string, supported format is [<count>,]<device-class>|<vendor/type>[,<key>=<value>;...]")

// DeviceType defines a device type.
type DeviceType string

// Constraints describes a set of device constraints.
type Constraints struct {

	// Type is the device type or device-class.
	// currently supported types are
	// - gpu
	// - nvidia.com/gpu
	// - amd.com/gpu
	Type DeviceType `bson:"type"`

	// Count is the number of devices that the user has asked for - count min and max are the
	// number of devices the charm requires.
	Count int64 `bson:"count"`

	// Attributes is a collection of key value pairs device related (node affinity labels/tags etc.).
	Attributes map[string]string `bson:"attributes"`
}

// ParseConstraints parses the specified string and creates a
// Constraints structure.
//
// The acceptable format for device constraints is a comma separated
// sequence of: COUNT, TYPE, and ATTRIBUTES with format like
//
//	<device-name>=[<count>,]<device-class>|<vendor/type>[,<attributes>]
//
// where
//
//	COUNT is the number of devices that the user has asked for - count min and max are the
//	number of devices the charm requires. If unspecified, COUNT defaults to 1.
func ParseConstraints(s string) (Constraints, error) {
	var cons Constraints

	fields := strings.Split(s, ",")
	fieldsLen := len(fields)
	if fieldsLen < 1 || fieldsLen > 3 {
		return cons, deviceParseErr
	}
	if fieldsLen == 1 {
		cons.Count = 1
		cons.Type = DeviceType(fields[0])
	} else {
		count, err := parseCount(fields[0])
		if err != nil {
			return Constraints{}, err
		}
		cons.Count = count
		cons.Type = DeviceType(fields[1])

		if fieldsLen == 3 {
			attr, err := parseAttributes(fields[2])
			if err != nil {
				return Constraints{}, err
			}
			cons.Attributes = attr
		}
	}
	return cons, nil
}

func parseAttributes(s string) (map[string]string, error) {
	parseAttribute := func(s string) ([]string, error) {
		kv := strings.Split(s, "=")
		if len(kv) != 2 {
			return nil, errors.Errorf("device attribute key/value pair has bad format: %q", s)
		}
		return kv, nil
	}
	attr := map[string]string{}
	for _, attrStr := range strings.Split(s, ";") {
		kv, err := parseAttribute(attrStr)
		if err != nil {
			return nil, err
		}
		attr[kv[0]] = kv[1]
	}
	return attr, nil
}

func parseCount(s string) (int64, error) {
	errMsg := errors.Errorf("count must be greater than zero, got %q", s)
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, errMsg
	}
	if i > 0 {
		return i, nil
	}
	return 0, errMsg
}
