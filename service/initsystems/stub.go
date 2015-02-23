// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package initsystems

import (
	"github.com/juju/testing"
)

// StubReturns holds the values returned by the various Stub methods.
type StubReturns struct {
	Name        string
	Names       []string
	Enabled     bool
	CheckPassed bool
	Info        ServiceInfo
	Conf        Conf
	ConfName    string
	Data        []byte
}

// Stub is used to simulate an init system without actually doing
// anything more than recording the calls.
type Stub struct {
	*testing.Stub

	Returns StubReturns
}

// NewStub creates a new Stub and returns it.
func NewStub() *Stub {
	return &Stub{
		Stub: &testing.Stub{},
	}
}

// Name implements InitSystem.
func (fi *Stub) Name() string {
	fi.AddCall("Name")
	fi.NextErr()
	return fi.Returns.Name
}

// List implements InitSystem.
func (fi *Stub) List(include ...string) ([]string, error) {
	fi.AddCall("List", include)
	return fi.Returns.Names, fi.NextErr()
}

// Start implements InitSystem.
func (fi *Stub) Start(name string) error {
	fi.AddCall("Start", name)
	return fi.NextErr()
}

// Stop implements InitSystem.
func (fi *Stub) Stop(name string) error {
	fi.AddCall("Stop", name)
	return fi.NextErr()
}

// Enable implements InitSystem.
func (fi *Stub) Enable(name, filename string) error {
	fi.AddCall("Enable", name)
	return fi.NextErr()
}

// Disable implements InitSystem.
func (fi *Stub) Disable(name string) error {
	fi.AddCall("Disable", name)
	return fi.NextErr()
}

// IsEnabled implements InitSystem.
func (fi *Stub) IsEnabled(name string) (bool, error) {
	fi.AddCall("IsEnabled", name)
	return fi.Returns.Enabled, fi.NextErr()
}

// Check implements InitSystem.
func (fi *Stub) Check(name, filename string) (bool, error) {
	fi.AddCall("Check", name, filename)
	return fi.Returns.CheckPassed, fi.NextErr()
}

// Info implements InitSystem.
func (fi *Stub) Info(name string) (ServiceInfo, error) {
	fi.AddCall("Info", name)
	return fi.Returns.Info, fi.NextErr()
}

// Conf implements InitSystem.
func (fi *Stub) Conf(name string) (Conf, error) {
	fi.AddCall("Conf", name)
	return fi.Returns.Conf, fi.NextErr()
}

// Validate implements InitSystem.
func (fi *Stub) Validate(name string, conf Conf) (string, error) {
	fi.AddCall("Validate", name, conf)
	return fi.Returns.ConfName, fi.NextErr()
}

// Serialize implements InitSystem.
func (fi *Stub) Serialize(name string, conf Conf) ([]byte, error) {
	fi.AddCall("Serialize", name, conf)
	return fi.Returns.Data, fi.NextErr()
}

// Deserialize implements InitSystem.
func (fi *Stub) Deserialize(data []byte, name string) (Conf, error) {
	fi.AddCall("Deserialize", data, name)
	return fi.Returns.Conf, fi.NextErr()
}
