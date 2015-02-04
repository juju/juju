// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package initsystems

import (
	"github.com/juju/testing"
)

// FakeReturns holds the values returned by the various Fake methods.
type FakeReturns struct {
	Name    string
	Names   []string
	Enabled bool
	Info    *ServiceInfo
	Conf    *Conf
	Data    []byte
}

// Fake is used to simulate an init system without actually doing
// anything more than recording the calls.
type Fake struct {
	testing.Fake

	Returns FakeReturns
}

// Name implements InitSystem.
func (fi *Fake) Name() string {
	fi.AddCall("Name", nil)
	fi.Err()
	return fi.Returns.Name
}

// List implements InitSystem.
func (fi *Fake) List(include ...string) ([]string, error) {
	fi.AddCall("List", testing.FakeCallArgs{
		"include": include,
	})
	return fi.Returns.Names, fi.Err()
}

// Start implements InitSystem.
func (fi *Fake) Start(name string) error {
	fi.AddCall("Start", testing.FakeCallArgs{
		"name": name,
	})
	return fi.Err()
}

// Stop implements InitSystem.
func (fi *Fake) Stop(name string) error {
	fi.AddCall("Stop", testing.FakeCallArgs{
		"name": name,
	})
	return fi.Err()
}

// Enable implements InitSystem.
func (fi *Fake) Enable(name, filename string) error {
	fi.AddCall("Enable", testing.FakeCallArgs{
		"name":     name,
		"filename": filename,
	})
	return fi.Err()
}

// Disable implements InitSystem.
func (fi *Fake) Disable(name string) error {
	fi.AddCall("Disable", testing.FakeCallArgs{
		"name": name,
	})
	return fi.Err()
}

// IsEnabled implements InitSystem.
func (fi *Fake) IsEnabled(name string) (bool, error) {
	fi.AddCall("IsEnabled", testing.FakeCallArgs{
		"name": name,
	})
	return fi.Returns.Enabled, fi.Err()
}

// Info implements InitSystem.
func (fi *Fake) Info(name string) (*ServiceInfo, error) {
	fi.AddCall("Info", testing.FakeCallArgs{
		"name": name,
	})
	return fi.Returns.Info, fi.Err()
}

// Conf implements InitSystem.
func (fi *Fake) Conf(name string) (*Conf, error) {
	fi.AddCall("Conf", testing.FakeCallArgs{
		"name": name,
	})
	return fi.Returns.Conf, fi.Err()
}

// Validate implements InitSystem.
func (fi *Fake) Validate(name string, conf Conf) error {
	fi.AddCall("Validate", testing.FakeCallArgs{
		"name": name,
		"conf": conf,
	})
	return fi.Err()
}

// Serialize implements InitSystem.
func (fi *Fake) Serialize(name string, conf Conf) ([]byte, error) {
	fi.AddCall("Serialize", testing.FakeCallArgs{
		"name": name,
		"conf": conf,
	})
	return fi.Returns.Data, fi.Err()
}

// Deserialize implements InitSystem.
func (fi *Fake) Deserialize(data []byte) (*Conf, error) {
	fi.AddCall("Deserialize", testing.FakeCallArgs{
		"data": data,
	})
	return fi.Returns.Conf, fi.Err()
}
