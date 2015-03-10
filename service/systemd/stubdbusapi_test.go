// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package systemd

import (
	"github.com/coreos/go-systemd/dbus"
	"github.com/juju/testing"
)

type StubDbusAPI struct {
	*testing.Stub

	Units     []dbus.UnitStatus
	Props     map[string]interface{}
	TypeProps map[string]interface{}
}

func (fda *StubDbusAPI) AddService(name, desc, status string) {
	active := ""
	load := "loaded"
	if status == "error" {
		load = status
	} else {
		active = status
	}

	unit := dbus.UnitStatus{
		Name:        name + ".service",
		Description: desc,
		ActiveState: active,
		LoadState:   load,
	}
	fda.Units = append(fda.Units, unit)
}

func (fda *StubDbusAPI) SetProperty(unitType, name string, value interface{}) {
	if unitType == "" {
		unitType = "Unit"
	}

	switch unitType {
	case "Unit":
		if fda.Props == nil {
			fda.Props = make(map[string]interface{})
		}
		fda.Props[name] = value
	default:
		if fda.TypeProps == nil {
			fda.TypeProps = make(map[string]interface{})
		}
		fda.TypeProps[name] = value
	}
}

func (fda *StubDbusAPI) ListUnits() ([]dbus.UnitStatus, error) {
	fda.Stub.AddCall("ListUnits")

	return fda.Units, fda.NextErr()
}

func (fda *StubDbusAPI) StartUnit(name string, mode string, ch chan<- string) (int, error) {
	fda.Stub.AddCall("StartUnit", name, mode, ch)

	return 0, fda.NextErr()
}

func (fda *StubDbusAPI) StopUnit(name string, mode string, ch chan<- string) (int, error) {
	fda.Stub.AddCall("StopUnit", name, mode, ch)

	return 0, fda.NextErr()
}

func (fda *StubDbusAPI) LinkUnitFiles(files []string, runtime bool, force bool) ([]dbus.LinkUnitFileChange, error) {
	fda.Stub.AddCall("LinkUnitFiles", files, runtime, force)

	return nil, fda.NextErr()
}

func (fda *StubDbusAPI) EnableUnitFiles(files []string, runtime bool, force bool) (bool, []dbus.EnableUnitFileChange, error) {
	fda.Stub.AddCall("EnableUnitFiles", files, runtime, force)

	return false, nil, fda.NextErr()
}

func (fda *StubDbusAPI) DisableUnitFiles(files []string, runtime bool) ([]dbus.DisableUnitFileChange, error) {
	fda.Stub.AddCall("DisableUnitFiles", files, runtime)

	return nil, fda.NextErr()
}

func (fda *StubDbusAPI) GetUnitProperties(unit string) (map[string]interface{}, error) {
	fda.Stub.AddCall("GetUnitProperties", unit)

	return fda.Props, fda.NextErr()
}

func (fda *StubDbusAPI) GetUnitTypeProperties(unit, unitType string) (map[string]interface{}, error) {
	fda.Stub.AddCall("GetUnitTypeProperties", unit, unitType)

	return fda.TypeProps, fda.NextErr()
}

func (fda *StubDbusAPI) Reload() error {
	fda.Stub.AddCall("Reload")

	return fda.Stub.NextErr()
}

func (fda *StubDbusAPI) Close() {
	fda.Stub.AddCall("Close")

	fda.Stub.NextErr() // We don't return the error (just pop it off).
}
