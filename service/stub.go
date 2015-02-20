// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/testing"
	"github.com/juju/utils/fs"
	"github.com/juju/utils/set"

	"github.com/juju/juju/service/initsystems"
)

// AddMockInitSystem registers a new mock InitSystem implementation.
// This is useful for testing.
func AddMockInitSystem(name, baseName string) {
	base := initsystems.NewInitSystem(baseName)
	if base == nil {
		panic(`unknown base init system "` + baseName + `"`)
	}
	fops := &fs.Ops{}
	newMock := func(name string) initsystems.InitSystem {
		return initsystems.NewTracking(base, fops)
	}

	initsystems.Register(name, initsystems.InitSystemDefinition{
		Name:        name,
		OSNames:     []string{"<any>"},
		Executables: []string{"<any>"},
		New:         newMock,
	})
}

// StubServiceStatus holds the sets of names for a given status.
type StubServicesStatus struct {
	Running set.Strings
	Enabled set.Strings
	Managed set.Strings
}

// StubServices is used in place of Services in testing.
type StubServices struct {
	*testing.Stub

	// Status is the collection of service statuses.
	Status StubServicesStatus

	// Confs tracks which confs have been passed to the methods.
	Confs map[string]Conf

	// Init is the init system name returned by InitSystem.
	Init string

	// CheckPassed is the list of return values for successive calls
	// to Check.
	CheckPassed []bool
}

// NewStubServices creates a new StubServices with the given init system
// name set.
func NewStubServices(init string) *StubServices {
	stub := &StubServices{
		Stub: &testing.Stub{},
		Init: init,
	}
	stub.Reset()
	return stub
}

// Reset sets the stub back to a pristine state.
func (fs *StubServices) Reset() {
	fs.Stub.Calls = nil
	fs.Status = StubServicesStatus{
		Running: set.NewStrings(),
		Enabled: set.NewStrings(),
		Managed: set.NewStrings(),
	}
	fs.Confs = make(map[string]Conf)
	fs.CheckPassed = nil
}

// InitSystem implements services.
func (fs *StubServices) InitSystem() string {
	fs.AddCall("InitSystem")

	fs.NextErr()
	return fs.Init
}

// List implements services.
func (fs *StubServices) List() ([]string, error) {
	fs.AddCall("List")

	return fs.Status.Managed.Values(), fs.NextErr()
}

// ListEnabled implements services.
func (fs *StubServices) ListEnabled() ([]string, error) {
	fs.AddCall("ListEnabled")

	return fs.Status.Enabled.Values(), fs.NextErr()
}

// Start implements services.
func (fs *StubServices) Start(name string) error {
	fs.AddCall("Start", name)

	fs.Status.Running.Add(name)
	return fs.NextErr()
}

// Stop implements services.
func (fs *StubServices) Stop(name string) error {
	fs.AddCall("Stop", name)

	fs.Status.Running.Remove(name)
	return fs.NextErr()
}

// IsRunning implements services.
func (fs *StubServices) IsRunning(name string) (bool, error) {
	fs.AddCall("IsRunning", name)

	return fs.Status.Running.Contains(name), fs.NextErr()
}

// Enable implements services.
func (fs *StubServices) Enable(name string) error {
	fs.AddCall("Enable", name)

	fs.Status.Enabled.Add(name)
	return fs.NextErr()
}

// Disable implements services.
func (fs *StubServices) Disable(name string) error {
	fs.AddCall("Disable", name)

	fs.Status.Running.Remove(name)
	fs.Status.Enabled.Remove(name)
	return fs.NextErr()
}

// IsEnabled implements services.
func (fs *StubServices) IsEnabled(name string) (bool, error) {
	fs.AddCall("IsEnabled", name)

	return fs.Status.Enabled.Contains(name), fs.NextErr()
}

// Manage implements services.
func (fs *StubServices) Manage(name string, conf Conf) error {
	fs.AddCall("Add", name, conf)

	if err := fs.NextErr(); err != nil {
		return err
	}
	if fs.Status.Managed.Contains(name) {
		return errors.AlreadyExistsf("service %q", name)
	}

	fs.Status.Managed.Add(name)
	fs.Confs[name] = conf
	return fs.NextErr()
}

// Remove implements services.
func (fs *StubServices) Remove(name string) error {
	fs.AddCall("Remove", name)

	fs.Status.Running.Remove(name)
	fs.Status.Enabled.Remove(name)
	fs.Status.Managed.Remove(name)
	return fs.NextErr()
}

// Check implements services.
func (fs *StubServices) Check(name string, conf Conf) (bool, error) {
	fs.AddCall("Check", name, conf)

	passed := true
	if len(fs.CheckPassed) > 0 {
		passed = fs.CheckPassed[0]
		fs.CheckPassed = fs.CheckPassed[1:]
	}
	return passed, fs.NextErr()
}

// IsManaged implements services.
func (fs *StubServices) IsManaged(name string) bool {
	fs.AddCall("IsManaged", name)

	return fs.Status.Managed.Contains(name)
}

// Install implements services.
func (fs *StubServices) Install(name string, conf Conf) error {
	fs.AddCall("Install", name, conf)

	fs.Status.Managed.Add(name)
	fs.Status.Enabled.Add(name)
	fs.Status.Running.Add(name)
	return fs.NextErr()
}

// NewAgentSevice implements services.
func (fs *StubServices) NewAgentService(tag names.Tag, paths AgentPaths, env map[string]string) (*Service, error) {
	fs.AddCall("NewAgentService", tag, paths, env)

	svc, _ := NewAgentService(tag, paths, env, fs)
	return svc, fs.NextErr()
}
