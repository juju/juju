// Copyright 2015 Cloudbase Solutions
// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !linux windows

package windows

import (
	"github.com/gabriel-samfira/sys/windows"
	"github.com/gabriel-samfira/sys/windows/svc"
	"github.com/gabriel-samfira/sys/windows/svc/mgr"

	"github.com/juju/testing"
)

// Unfortunately this cannot be moved inside StubMgr because the Delete() method
// is attached to the service itself
var Services map[string]*StubService

type StubService struct {
	*testing.Stub

	Name      string
	config    mgr.Config
	ExecStart string
	Closed    bool

	Status svc.Status
}

func AddService(name, execStart string, stub *testing.Stub, status svc.Status) {
	Services[name] = &StubService{
		Stub:      stub,
		Name:      name,
		ExecStart: execStart,
		config:    mgr.Config{},
		Status:    status,
	}
}

func (s *StubService) Close() error {
	s.Stub.AddCall("Close")
	s.Closed = true
	return s.NextErr()
}

func (s *StubService) UpdateConfig(c mgr.Config) error {
	s.config = c
	return s.NextErr()
}

func (s *StubService) SetStatus(status svc.Status) {
	s.Status = status
}

func (s *StubService) Config() (mgr.Config, error) {
	return s.config, s.NextErr()
}

func (s *StubService) Control(c svc.Cmd) (svc.Status, error) {
	s.Stub.AddCall("Control", c)

	switch c {
	case svc.Interrogate:
	case svc.Stop:
		s.Status = svc.Status{State: svc.Stopped}
	case svc.Pause:
		s.Status = svc.Status{State: svc.Paused}
	case svc.Continue:
		s.Status = svc.Status{State: svc.Running}
	case svc.Shutdown:
		s.Status = svc.Status{State: svc.Stopped}
	}
	return s.Status, s.NextErr()
}

func (s *StubService) Delete() error {
	s.Stub.AddCall("Control")

	if _, ok := Services[s.Name]; ok {
		delete(Services, s.Name)
		return s.NextErr()
	}
	return c_ERROR_SERVICE_DOES_NOT_EXIST
}

func (s *StubService) Query() (svc.Status, error) {
	s.Stub.AddCall("Query")

	return s.Status, s.NextErr()
}

func (s *StubService) Start(args ...string) error {
	s.Stub.AddCall("Start", args)

	s.Status = svc.Status{State: svc.Running}
	return s.NextErr()
}

type StubMgr struct {
	*testing.Stub
}

func (m *StubMgr) CreateService(name, exepath string, c mgr.Config, args ...string) (windowsService, error) {
	m.Stub.AddCall("CreateService", name, exepath, c)

	if _, ok := Services[name]; ok {
		return nil, c_ERROR_SERVICE_EXISTS
	}
	stubSvc := &StubService{
		Name:      name,
		ExecStart: exepath,
		config:    c,
		Status:    svc.Status{State: svc.Stopped},
		Stub:      m.Stub,
	}
	Services[name] = stubSvc
	return stubSvc, m.NextErr()
}

func (m *StubMgr) Disconnect() error {
	m.Stub.AddCall("Disconnect")
	return m.NextErr()
}

func (m *StubMgr) OpenService(name string) (windowsService, error) {
	m.Stub.AddCall("OpenService", name)
	if stubSvc, ok := Services[name]; ok {
		return stubSvc, m.NextErr()
	}
	return nil, c_ERROR_SERVICE_DOES_NOT_EXIST
}

func (m *StubMgr) GetHandle(name string) (handle windows.Handle, err error) {
	m.Stub.AddCall("GetHandle", name)
	if _, ok := Services[name]; ok {
		return handle, m.NextErr()
	}
	return handle, c_ERROR_SERVICE_DOES_NOT_EXIST
}

func (m *StubMgr) CloseHandle(handle windows.Handle) (err error) {
	m.Stub.AddCall("CloseHandle")
	return m.NextErr()
}

func (m *StubMgr) Exists(name string) bool {
	if _, ok := Services[name]; ok {
		return true
	}
	return false
}

func (m *StubMgr) List() []string {
	svcs := []string{}
	for i := range Services {
		svcs = append(svcs, i)
	}
	return svcs
}

func (m *StubMgr) Clear() {
	Services = map[string]*StubService{}
}
