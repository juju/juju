// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/testing"
	"github.com/juju/utils/set"
)

// FakeServiceStatus holds the sets of names for a given status.
type FakeServicesStatus struct {
	Running set.Strings
	Enabled set.Strings
	Managed set.Strings
}

// FakeServices is used in place of Services in testing.
type FakeServices struct {
	*testing.Fake

	// Status is the collection of service statuses.
	Status FakeServicesStatus

	// Confs tracks which confs have been passed to the methods.
	Confs map[string]Conf

	// Init is the init system name returned by InitSystem.
	Init string

	// CheckPassed is the list of return values for successive calls
	// to Check.
	CheckPassed []bool
}

// NewFakeServices creates a new FakeServices with the given init system
// name set.
func NewFakeServices(init string) *FakeServices {
	fake := &FakeServices{
		Fake: &testing.Fake{},
		Init: init,
	}
	fake.Reset()
	return fake
}

// Reset sets the fake back to a pristine state.
func (fs *FakeServices) Reset() {
	fs.Fake.Reset()
	fs.Status = FakeServicesStatus{
		Running: set.NewStrings(),
		Enabled: set.NewStrings(),
		Managed: set.NewStrings(),
	}
	fs.Confs = make(map[string]Conf)
	fs.CheckPassed = nil
}

// InitSystem implements services.
func (fs *FakeServices) InitSystem() string {
	fs.AddCall("InitSystem", nil)

	fs.Err()
	return fs.Init
}

// List implements services.
func (fs *FakeServices) List() ([]string, error) {
	fs.AddCall("List", nil)

	return fs.Status.Managed.Values(), fs.Err()
}

// ListEnabled implements services.
func (fs *FakeServices) ListEnabled() ([]string, error) {
	fs.AddCall("ListEnabled", nil)

	return fs.Status.Enabled.Values(), fs.Err()
}

// Start implements services.
func (fs *FakeServices) Start(name string) error {
	fs.AddCall("Start", testing.FakeCallArgs{
		"name": name,
	})

	fs.Status.Running.Add(name)
	return fs.Err()
}

// Stop implements services.
func (fs *FakeServices) Stop(name string) error {
	fs.AddCall("Stop", testing.FakeCallArgs{
		"name": name,
	})

	fs.Status.Running.Remove(name)
	return fs.Err()
}

// IsRunning implements services.
func (fs *FakeServices) IsRunning(name string) (bool, error) {
	fs.AddCall("IsRunning", testing.FakeCallArgs{
		"name": name,
	})

	return fs.Status.Running.Contains(name), fs.Err()
}

// Enable implements services.
func (fs *FakeServices) Enable(name string) error {
	fs.AddCall("Enable", testing.FakeCallArgs{
		"name": name,
	})

	fs.Status.Enabled.Add(name)
	return fs.Err()
}

// Disable implements services.
func (fs *FakeServices) Disable(name string) error {
	fs.AddCall("Disable", testing.FakeCallArgs{
		"name": name,
	})

	fs.Status.Running.Remove(name)
	fs.Status.Enabled.Remove(name)
	return fs.Err()
}

// IsEnabled implements services.
func (fs *FakeServices) IsEnabled(name string) (bool, error) {
	fs.AddCall("IsEnabled", testing.FakeCallArgs{
		"name": name,
	})

	return fs.Status.Enabled.Contains(name), fs.Err()
}

// Manage implements services.
func (fs *FakeServices) Manage(name string, conf Conf) error {
	fs.AddCall("Add", testing.FakeCallArgs{
		"name": name,
		"conf": conf,
	})

	if err := fs.Err(); err != nil {
		return err
	}
	if fs.Status.Managed.Contains(name) {
		return errors.AlreadyExistsf("service %q", name)
	}

	fs.Status.Managed.Add(name)
	fs.Confs[name] = conf
	return fs.Err()
}

// Remove implements services.
func (fs *FakeServices) Remove(name string) error {
	fs.AddCall("Remove", testing.FakeCallArgs{
		"name": name,
	})

	fs.Status.Running.Remove(name)
	fs.Status.Enabled.Remove(name)
	fs.Status.Managed.Remove(name)
	return fs.Err()
}

// Check implements services.
func (fs *FakeServices) Check(name string, conf Conf) (bool, error) {
	fs.AddCall("Check", testing.FakeCallArgs{
		"name": name,
		"conf": conf,
	})

	passed := true
	if len(fs.CheckPassed) > 0 {
		passed = fs.CheckPassed[0]
		fs.CheckPassed = fs.CheckPassed[1:]
	}
	return passed, fs.Err()
}

// IsManaged implements services.
func (fs *FakeServices) IsManaged(name string) bool {
	fs.AddCall("IsManaged", testing.FakeCallArgs{
		"name": name,
	})

	return fs.Status.Managed.Contains(name)
}

// Install implements services.
func (fs *FakeServices) Install(name string, conf Conf) error {
	fs.AddCall("Install", testing.FakeCallArgs{
		"name": name,
		"conf": conf,
	})

	fs.Status.Managed.Add(name)
	fs.Status.Enabled.Add(name)
	fs.Status.Running.Add(name)
	return fs.Err()
}

// NewAgentSevice implements services.
func (fs *FakeServices) NewAgentService(tag names.Tag, paths AgentPaths, env map[string]string) (*Service, error) {
	fs.AddCall("NewAgentService", testing.FakeCallArgs{
		"tag":   tag,
		"paths": paths,
		"env":   env,
	})

	svc, _ := NewAgentService(tag, paths, env, fs)
	return svc, fs.Err()
}
