// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package systemd

import (
	"fmt"

	"github.com/coreos/go-systemd/dbus"
	"github.com/juju/errors"
)

// listUnits returns a list of UnitStatus structures of all loaded units
func listUnits() ([]dbus.UnitStatus, error) {
	conn, err := dbus.New()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	return conn.ListUnits()
}

// reloadDaemon signals systemd to reload all unit files.
func reloadDaemon() error {
	conn, err := dbus.New()
	if err != nil {
		return err
	}
	defer conn.Close()

	return conn.Reload()
}

// enableUnit issues the command to enable the given unit.
// it may take an absolute path to the unit file if it lies outside of
// systemd's search path.
func enableUnit(name string) error {
	conn, err := dbus.New()
	if err != nil {
		return err
	}
	defer conn.Close()

	_, _, err = conn.EnableUnitFiles([]string{name}, false, true)

	return err
}

// disableUnit issues the command to disable the specified unit.
// it may take an absolute path to the unit file if it lies outside of
// systemd's search path.
func disableUnit(name string) error {
	conn, err := dbus.New()
	if err != nil {
		return err
	}
	defer conn.Close()

	_, err = conn.DisableUnitFiles([]string{name}, false)

	return err
}

// startUnit asks systemd to start the unit identified by the given name
func startUnit(name string) error {
	conn, err := dbus.New()
	if err != nil {
		return err
	}
	defer conn.Close()

	ch := make(chan string)

	_, err = conn.StartUnit(name, "fail", ch)
	if err != nil {
		return err
	}

	// wait for the unit to start
	status := <-ch
	if status != "done" {
		return errors.New(fmt.Sprintf("unit %s has failed to start", name))
	}

	return nil
}

// stopUnit asks systemd to stop the unit identified by the given name
func stopUnit(name string) error {
	conn, err := dbus.New()
	if err != nil {
		return err
	}
	defer conn.Close()

	ch := make(chan string)

	_, err = conn.StopUnit(name, "fail", ch)
	if err != nil {
		return err
	}

	// wait for the unit to stop
	status := <-ch
	if status != "done" {
		return errors.New(fmt.Sprintf("unit %s has failed to stop", name))
	}

	return nil
}
