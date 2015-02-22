// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package systemd

import (
	"strconv"
	"strings"

	"github.com/coreos/go-systemd/unit"
	"github.com/juju/errors"
)

var installProperties = []string{
	"Alias",
	"WantedBy",
	"RequiredBy",
	"Also",
	"DefaultInstance",
}

func getUnitOptions(conn dbusAPI, name, unitType string) ([]*unit.UnitOption, error) {
	var opts []*unit.UnitOption

	// Extract the generic properties first.
	props, err := conn.GetUnitProperties(name)
	if err != nil {
		return opts, errors.Trace(err)
	}
	opts = parseProperties(props, "Unit")

	// Extract the unit type properties.
	props, err = conn.GetUnitTypeProperties(name, unitType)
	if err != nil {
		return opts, errors.Trace(err)
	}
	typeOpts := parseProperties(props, unitType)

	return append(opts, typeOpts...), nil
}

func parseProperties(props map[string]interface{}, unitType string) []*unit.UnitOption {
	// See:
	//  http://dbus.freedesktop.org/doc/dbus-specification.html#basic-types
	//  http://www.freedesktop.org/wiki/Software/systemd/dbus/

	var parseProperty func(string, interface{}) []*unit.UnitOption
	switch unitType {
	case "Unit", "":
		parseProperty = parseUnitProperty
	case "Service":
		parseProperty = parseServiceProperty
	default:
		return nil
	}

	var opts []*unit.UnitOption
	for key, value := range props {
		opts = append(opts, parseProperty(key, value)...)
	}
	return opts
}

func parseUnitProperty(name string, value interface{}) []*unit.UnitOption {
	var opts []*unit.UnitOption

	section := "Unit"
	for _, installKey := range installProperties {
		if name == installKey {
			section = "Install"
			break
		}
	}

	// We only support the values we need.
	var strValues []string
	switch name {
	case "Description":
		strValue, _ := value.(string)
		strValues = append(strValues, strValue)
	}

	for _, strValue := range strValues {
		opt := &unit.UnitOption{
			Section: section,
			Name:    name,
			Value:   strValue,
		}
		opts = append(opts, opt)
	}

	return opts
}

func parseServiceProperty(name string, value interface{}) []*unit.UnitOption {
	var opts []*unit.UnitOption

	// We only support the values we need.
	var strValues []string
	switch {
	case name == "ExecStart":
		if parts, ok := value.([]interface{}); ok {
			if len(parts) < 2 {
				return nil
			}
			cmd, ok := parts[0].(string)
			if !ok {
				return nil
			}
			args, ok := parts[1].([]interface{})
			if !ok {
				return nil
			}
			for _, arg := range args {
				strArg, ok := arg.(string)
				if !ok {
					cmd = ""
					break
				}
				cmd += " " + strArg
			}

			if cmd != "" {
				strValues = append(strValues, cmd)
			}
		}
	case name == "StandardOutput":
		if strValue, ok := value.(string); ok {
			strValues = append(strValues, strValue)
		}
	case name == "StandardError":
		if strValue, ok := value.(string); ok {
			strValues = append(strValues, strValue)
		}
	case name == "Environment":
		if envValues, ok := value.([]interface{}); ok {
			for _, val := range envValues {
				if strValue, ok := val.(string); ok {
					strValues = append(strValues, strValue)
				}
			}
		}
	case strings.HasPrefix(name, "Limit"):
		if intValue, ok := value.(uint64); ok {
			strValue := strconv.FormatUint(intValue, 10)
			strValues = append(strValues, strValue)
		}
	}

	for _, strValue := range strValues {
		opt := &unit.UnitOption{
			Section: "Service",
			Name:    name,
			Value:   strValue,
		}
		opts = append(opts, opt)
	}

	return opts
}
