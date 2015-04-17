// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"strings"

	"github.com/juju/errors"
	"github.com/juju/testing"
	"github.com/juju/utils/set"

	"github.com/juju/juju/service/common"
)

type serviceInfo interface {
	Name() string
	Conf() common.Conf
}

// FakeServiceData holds the results of Service method calls.
type FakeServiceData struct {
	*testing.Stub

	// Installed is the list of all services that were installed.
	Installed []serviceInfo

	// Removed is the list of all services that were removed.
	Removed []serviceInfo

	// ManagedNames is the set of "currently" juju-managed services.
	ManagedNames set.Strings

	// InstalledNames is the set of "currently" installed services.
	InstalledNames set.Strings

	// RunningNames is the set of "currently" running services.
	RunningNames set.Strings

	// InstallCommands is the value to return for Service.InstallCommands.
	InstallCommands []string

	// StartCommands is the value to return for Service.StartCommands.
	StartCommands []string
}

// NewFakeServiceData returns a new FakeServiceData.
func NewFakeServiceData() *FakeServiceData {
	return &FakeServiceData{
		Stub:           &testing.Stub{},
		ManagedNames:   set.NewStrings(),
		InstalledNames: set.NewStrings(),
		RunningNames:   set.NewStrings(),
	}
}

// SetStatus updates the status of the named service.
func (fsd *FakeServiceData) SetStatus(name, status string) error {
	if status == "" {
		fsd.ManagedNames.Remove(name)
		fsd.InstalledNames.Remove(name)
		fsd.RunningNames.Remove(name)
		return nil
	}

	managed := true
	if strings.HasPrefix(status, "(") && strings.HasSuffix(status, ")") {
		status = status[1 : len(status)-1]
		managed = false
	}

	switch status {
	case "installed":
		fsd.InstalledNames.Add(name)
		fsd.RunningNames.Remove(name)
	case "running":
		fsd.InstalledNames.Add(name)
		fsd.RunningNames.Add(name)
	default:
		return errors.NotSupportedf("status %q", status)
	}

	if managed {
		fsd.ManagedNames.Add(name)
	}
	return nil
}

// FakeService is a Service implementation for testing.
type FakeService struct {
	*FakeServiceData
	common.Service
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

	ss.NextErr()
	return ss.Service.Name
}

// Conf implements Service.
func (ss *FakeService) Conf() common.Conf {
	ss.AddCall("Conf")

	ss.NextErr()
	return ss.Service.Conf
}

// Running implements Service.
func (ss *FakeService) Running() (bool, error) {
	ss.AddCall("Running")

	return ss.running(), ss.NextErr()
}

func (ss *FakeService) running() bool {
	return ss.FakeServiceData.RunningNames.Contains(ss.Service.Name)
}

// Start implements Service.
func (ss *FakeService) Start() error {
	ss.AddCall("Start")
	// TODO(ericsnow) Check managed?
	if ss.running() {
		ss.FakeServiceData.RunningNames.Add(ss.Service.Name)
	}

	return ss.NextErr()
}

// Stop implements Service.
func (ss *FakeService) Stop() error {
	ss.AddCall("Stop")
	if !ss.running() {
		ss.FakeServiceData.RunningNames.Remove(ss.Service.Name)
	}

	return ss.NextErr()
}

// Exists implements Service.
func (ss *FakeService) Exists() (bool, error) {
	ss.AddCall("Exists")

	return ss.managed(), ss.NextErr()
}

func (ss *FakeService) managed() bool {
	return ss.FakeServiceData.ManagedNames.Contains(ss.Service.Name)
}

// Installed implements Service.
func (ss *FakeService) Installed() (bool, error) {
	ss.AddCall("Installed")

	return ss.installed(), ss.NextErr()
}

func (ss *FakeService) installed() bool {
	return ss.FakeServiceData.InstalledNames.Contains(ss.Service.Name)
}

// Install implements Service.
func (ss *FakeService) Install() error {
	ss.AddCall("Install")
	if !ss.running() && !ss.installed() {
		ss.FakeServiceData.Installed = append(ss.FakeServiceData.Installed, ss)
		ss.FakeServiceData.InstalledNames.Add(ss.Service.Name)
	}

	return ss.NextErr()
}

// Remove implements Service.
func (ss *FakeService) Remove() error {
	ss.AddCall("Remove")
	if ss.installed() {
		ss.FakeServiceData.Removed = append(ss.FakeServiceData.Removed, ss)
		ss.FakeServiceData.InstalledNames.Remove(ss.Service.Name)
	}

	return ss.NextErr()
}

// InstallCommands implements Service.
func (ss *FakeService) InstallCommands() ([]string, error) {
	ss.AddCall("InstallCommands")

	return ss.FakeServiceData.InstallCommands, ss.NextErr()
}

// StartCommands implements Service.
func (ss *FakeService) StartCommands() ([]string, error) {
	ss.AddCall("StartCommands")

	return ss.FakeServiceData.StartCommands, ss.NextErr()
}
