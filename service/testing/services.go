// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/testing"
	"github.com/juju/utils/set"

	"github.com/juju/juju/service"
)

// ServiceStatus holds the sets of names for a given status.
type ServicesStatus struct {
	Running set.Strings
	Enabled set.Strings
	Managed set.Strings
}

// FakeServices is used in place of service.Services in testing.
type FakeServices struct {
	testing.Fake

	// Status is the collection of service statuses.
	Status ServicesStatus
	// Confs tracks which confs have been passed to the methods.
	Confs map[string]service.Conf
	// CheckPassed is the list of return values for successive calls
	// to Check.
	CheckPassed []bool

	init string
}

// NewFakeServices creates a new FakeServices with the given init system
// name set.
func NewFakeServices(init string) *FakeServices {
	return &FakeServices{
		init:  init,
		Confs: make(map[string]service.Conf),
		Status: ServicesStatus{
			Running: set.NewStrings(),
			Enabled: set.NewStrings(),
			Managed: set.NewStrings(),
		},
	}
}

// InitSystem implements service.services.
func (fs *FakeServices) InitSystem() string {
	fs.AddCall("InitSystem", nil)

	fs.Err()
	return fs.init
}

// List implements service.services.
func (fs *FakeServices) List() ([]string, error) {
	fs.AddCall("List", nil)

	return fs.Status.Managed.Values(), fs.Err()
}

// ListEnabled implements service.services.
func (fs *FakeServices) ListEnabled() ([]string, error) {
	fs.AddCall("ListEnabled", nil)

	return fs.Status.Enabled.Values(), fs.Err()
}

// Start implements service.services.
func (fs *FakeServices) Start(name string) error {
	fs.AddCall("Start", testing.FakeCallArgs{
		"name": name,
	})

	fs.Status.Running.Add(name)
	return fs.Err()
}

// Stop implements service.services.
func (fs *FakeServices) Stop(name string) error {
	fs.AddCall("Stop", testing.FakeCallArgs{
		"name": name,
	})

	fs.Status.Running.Remove(name)
	return fs.Err()
}

// IsRunning implements service.services.
func (fs *FakeServices) IsRunning(name string) (bool, error) {
	fs.AddCall("IsRunning", testing.FakeCallArgs{
		"name": name,
	})

	return fs.Status.Running.Contains(name), fs.Err()
}

// Enable implements service.services.
func (fs *FakeServices) Enable(name string) error {
	fs.AddCall("Enable", testing.FakeCallArgs{
		"name": name,
	})

	fs.Status.Enabled.Add(name)
	return fs.Err()
}

// Disable implements service.services.
func (fs *FakeServices) Disable(name string) error {
	fs.AddCall("Disable", testing.FakeCallArgs{
		"name": name,
	})

	fs.Status.Running.Remove(name)
	fs.Status.Enabled.Remove(name)
	return fs.Err()
}

// IsEnabled implements service.services.
func (fs *FakeServices) IsEnabled(name string) (bool, error) {
	fs.AddCall("IsEnabled", testing.FakeCallArgs{
		"name": name,
	})

	return fs.Status.Enabled.Contains(name), fs.Err()
}

// Manage implements service.services.
func (fs *FakeServices) Manage(name string, conf service.Conf) error {
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

// Remove implements service.services.
func (fs *FakeServices) Remove(name string) error {
	fs.AddCall("Remove", testing.FakeCallArgs{
		"name": name,
	})

	fs.Status.Running.Remove(name)
	fs.Status.Enabled.Remove(name)
	fs.Status.Managed.Remove(name)
	return fs.Err()
}

// Check implements service.services.
func (fs *FakeServices) Check(name string, conf service.Conf) (bool, error) {
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

// IsManaged implements service.services.
func (fs *FakeServices) IsManaged(name string) bool {
	fs.AddCall("IsManaged", testing.FakeCallArgs{
		"name": name,
	})

	return fs.Status.Managed.Contains(name)
}

// Install implements service.services.
func (fs *FakeServices) Install(name string, conf service.Conf) error {
	fs.AddCall("Install", testing.FakeCallArgs{
		"name": name,
		"conf": conf,
	})

	fs.Status.Managed.Add(name)
	fs.Status.Enabled.Add(name)
	fs.Status.Running.Add(name)
	return fs.Err()
}

// NewAgentSevice implements service.services.
func (fs *FakeServices) NewAgentService(tag names.Tag, paths service.AgentPaths, env map[string]string) (*service.Service, error) {
	fs.AddCall("NewAgentService", testing.FakeCallArgs{
		"tag":   tag,
		"paths": paths,
		"env":   env,
	})

	svc, _ := service.NewAgentService(tag, paths, env, fs)
	return svc, fs.Err()
}
