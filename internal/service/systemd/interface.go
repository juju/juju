// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package systemd

import (
	"os"

	"github.com/coreos/go-systemd/v22/dbus"
)

// DBusAPI describes all the systemd API methods needed by juju.
type DBusAPI interface {
	Close()
	ListUnits() ([]dbus.UnitStatus, error)
	StartUnit(string, string, chan<- string) (int, error)
	StopUnit(string, string, chan<- string) (int, error)
	LinkUnitFiles([]string, bool, bool) ([]dbus.LinkUnitFileChange, error)
	EnableUnitFiles([]string, bool, bool) (bool, []dbus.EnableUnitFileChange, error)
	DisableUnitFiles([]string, bool) ([]dbus.DisableUnitFileChange, error)
	GetUnitProperties(string) (map[string]interface{}, error)
	GetUnitTypeProperties(string, string) (map[string]interface{}, error)
	Reload() error
}

// FileSystemOps describes file-system operations required to install
// and remove a service.
type FileSystemOps interface {
	Remove(name string) error
	RemoveAll(name string) error
	WriteFile(fileName string, data []byte, perm os.FileMode) error
}
