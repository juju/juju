// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"strings"
	"sync"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/testing"

	"github.com/juju/juju/v3/service/common"
)

type ServiceInfo interface {
	Name() string
	Conf() common.Conf
}

// FakeServiceData holds the results of Service method calls.
type FakeServiceData struct {
	testing.Stub

	mu sync.Mutex

	// installed is the list of all services that were installed.
	installed []ServiceInfo

	// removed is the list of all services that were removed.
	removed []ServiceInfo

	// managedNames is the set of "currently" juju-managed services.
	managedNames set.Strings

	// installedNames is the set of "currently" installed services.
	installedNames set.Strings

	// runningNames is the set of "currently" running services.
	runningNames set.Strings

	// differentNames is the set of services where the content is different.
	differentNames set.Strings
}

// NewFakeServiceData returns a new FakeServiceData.
func NewFakeServiceData(names ...string) *FakeServiceData {
	fsd := FakeServiceData{
		managedNames:   set.NewStrings(),
		installedNames: set.NewStrings(),
		runningNames:   set.NewStrings(),
		differentNames: set.NewStrings(),
	}
	for _, name := range names {
		fsd.installedNames.Add(name)
	}
	return &fsd
}

// InstalledNames returns a copy of the list of the installed names.
func (f *FakeServiceData) InstalledNames() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.installedNames.Values()
}

// Installed returns a copy of the list of installed Services
func (f *FakeServiceData) Installed() []ServiceInfo {
	f.mu.Lock()
	defer f.mu.Unlock()
	names := make([]ServiceInfo, len(f.installed))
	copy(names, f.installed)
	return names
}

// GetInstalled returns the installed service that matches name.

// Removed returns a copy of the list of removed Services
func (f *FakeServiceData) Removed() []ServiceInfo {
	f.mu.Lock()
	defer f.mu.Unlock()
	names := make([]ServiceInfo, len(f.removed))
	copy(names, f.removed)
	return names
}

// GetInstalled returns the installed service that matches name.
// If name is not found, the method panics.
func (f *FakeServiceData) GetInstalled(name string) ServiceInfo {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, i := range f.installed {
		if i.Name() == name {
			return i
		}
	}
	panic(name + " not found")
}

// SetStatus updates the status of the named service.
func (f *FakeServiceData) SetStatus(name, status string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if status == "" {
		f.managedNames.Remove(name)
		f.installedNames.Remove(name)
		f.runningNames.Remove(name)
		return nil
	}

	managed := true
	if strings.HasPrefix(status, "(") && strings.HasSuffix(status, ")") {
		status = status[1 : len(status)-1]
		managed = false
	}

	switch status {
	case "installed":
		f.installedNames.Add(name)
		f.runningNames.Remove(name)
	case "running":
		f.installedNames.Add(name)
		f.runningNames.Add(name)
	case "different":
		f.installedNames.Add(name)
		f.runningNames.Add(name)
		f.differentNames.Add(name)
	default:
		return errors.NotSupportedf("status %q", status)
	}

	if managed {
		f.managedNames.Add(name)
	}
	return nil
}

// FakeService is a Service implementation for testing.
type FakeService struct {
	*FakeServiceData
	common.Service

	DataDir string
}

// NewFakeService returns a new FakeService.
func NewFakeService(name string, conf common.Conf) *FakeService {
	return &FakeService{
		FakeServiceData: NewFakeServiceData(),
		Service: common.Service{
			Name: name,
			Conf: conf,
		},
	}
}

// Name implements Service.
func (ss *FakeService) Name() string {
	ss.AddCall("Name")

	return ss.Service.Name
}

// Conf implements Service.
func (ss *FakeService) Conf() common.Conf {
	ss.AddCall("Conf")

	_ = ss.NextErr()
	return ss.Service.Conf
}

// Running implements Service.
func (ss *FakeService) Running() (bool, error) {
	return ss.running(), ss.simpleCall("Running")
}

func (ss *FakeService) running() bool {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return ss.FakeServiceData.runningNames.Contains(ss.Service.Name)
}

// Start implements Service.
func (ss *FakeService) Start() error {
	ss.AddCall("Start", ss.Service.Name)
	// TODO(ericsnow) Check managed?
	if !ss.running() {
		ss.mu.Lock()
		ss.FakeServiceData.runningNames.Add(ss.Service.Name)
		ss.mu.Unlock()
	}

	return ss.NextErr()
}

// Stop implements Service.
func (ss *FakeService) Stop() error {
	ss.AddCall("Stop")
	if ss.running() {
		ss.mu.Lock()
		ss.FakeServiceData.runningNames.Remove(ss.Service.Name)
		ss.mu.Unlock()
	}

	return ss.NextErr()
}

// Exists implements Service.
func (ss *FakeService) Exists() (bool, error) {
	ss.AddCall("Exists")

	managed := ss.managed()
	ss.mu.Lock()
	different := ss.FakeServiceData.differentNames.Contains(ss.Service.Name)
	ss.mu.Unlock()

	return managed && !different, ss.NextErr()
}

func (ss *FakeService) managed() bool {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return ss.FakeServiceData.managedNames.Contains(ss.Service.Name)
}

// Installed implements Service.
func (ss *FakeService) Installed() (bool, error) {
	return ss.installed(), ss.simpleCall("Installed")
}

func (ss *FakeService) installed() bool {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return ss.FakeServiceData.installedNames.Contains(ss.Service.Name)
}

// Install implements Service.
func (ss *FakeService) Install() error {
	ss.AddCall("Install")
	if !ss.running() && !ss.installed() {
		ss.mu.Lock()
		ss.FakeServiceData.installed = append(ss.FakeServiceData.installed, ss)
		ss.FakeServiceData.installedNames.Add(ss.Service.Name)
		ss.mu.Unlock()
	}

	return ss.NextErr()
}

// Remove implements Service.
func (ss *FakeService) Remove() error {
	ss.AddCall("Remove")
	if ss.installed() {
		ss.mu.Lock()
		ss.FakeServiceData.removed = append(ss.FakeServiceData.removed, ss)
		ss.FakeServiceData.installedNames.Remove(ss.Service.Name)
		ss.mu.Unlock()
	}

	return ss.NextErr()
}

// InstallCommands implements Service.
func (ss *FakeService) InstallCommands() ([]string, error) {
	return nil, ss.simpleCall("InstallCommands")
}

// StartCommands implements Service.
func (ss *FakeService) StartCommands() ([]string, error) {
	return nil, ss.simpleCall("StartCommands")
}

// WriteService implements UpgradableService.
func (ss *FakeService) WriteService() error {
	return ss.simpleCall("WriteService")
}

// RemoveOldService implements UpgradableService.
func (ss *FakeService) RemoveOldService() error {
	return ss.simpleCall("RemoveOldService")
}

func (ss *FakeService) simpleCall(methodName string) error {
	ss.AddCall(methodName)
	return ss.NextErr()
}
