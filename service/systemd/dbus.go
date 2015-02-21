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

	// See:
	//  http://dbus.freedesktop.org/doc/dbus-specification.html#basic-types
	//  http://www.freedesktop.org/wiki/Software/systemd/dbus/

	// Extract the generic properties first.
	props, err := conn.GetUnitProperties(name)
	if err != nil {
		return opts, errors.Trace(err)
	}
	for key, value := range props {
		section := "Unit"
		for _, installKey := range installProperties {
			if key == installKey {
				section = "Install"
				break
			}
		}

		// We only support the values we need.
		var strValues []string
		switch key {
		case "Description":
			strValue, _ := value.(string)
			strValues = append(strValues, strValue)
		}

		for _, strValue := range strValues {
			opt := &unit.UnitOption{
				Section: section,
				Name:    key,
				Value:   strValue,
			}
			opts = append(opts, opt)
		}
	}

	props, err = conn.GetUnitTypeProperties(name, unitType)
	if err != nil {
		return opts, errors.Trace(err)
	}
	for key, value := range props {
		// We only support the values we need.
		var strValues []string
		switch {
		case key == "ExecStart":
			if parts, ok := value.([]interface{}); ok {
				if len(parts) < 2 {
					continue
				}
				cmd, ok := parts[0].(string)
				if !ok {
					continue
				}
				args, ok := parts[1].([]interface{})
				if !ok {
					continue
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
		case key == "StandardOutput":
			if strValue, ok := value.(string); ok {
				strValues = append(strValues, strValue)
			}
		case key == "StandardError":
			if strValue, ok := value.(string); ok {
				strValues = append(strValues, strValue)
			}
		case key == "Environment":
			if envValues, ok := value.([]interface{}); ok {
				for _, val := range envValues {
					if strValue, ok := val.(string); ok {
						strValues = append(strValues, strValue)
					}
				}
			}
		case strings.HasPrefix(key, "Limit"):
			if intValue, ok := value.(uint64); ok {
				strValue := strconv.FormatUint(intValue, 10)
				strValues = append(strValues, strValue)
			}
		}

		for _, strValue := range strValues {
			opt := &unit.UnitOption{
				Section: unitType,
				Name:    key,
				Value:   strValue,
			}
			opts = append(opts, opt)
		}
	}

	return opts, nil
}
