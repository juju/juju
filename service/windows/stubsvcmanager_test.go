package windows

import (
	"github.com/juju/testing"

	"github.com/juju/juju/service/common"
)

type service struct {
	running bool

	conf common.Conf
}

var MgrServices = map[string]*service{}

type StubSvcManager struct {
	*testing.Stub
}

func (s *StubSvcManager) Start(name string) error {
	s.Stub.AddCall("Start", name)

	if svc, ok := MgrServices[name]; !ok {
		return c_ERROR_SERVICE_DOES_NOT_EXIST
	} else {
		svc.running = true
	}
	return nil
}

func (s *StubSvcManager) Stop(name string) error {
	s.Stub.AddCall("Stop", name)

	if svc, ok := MgrServices[name]; !ok {
		return c_ERROR_SERVICE_DOES_NOT_EXIST
	} else {
		svc.running = false
	}
	return nil
}

func (s *StubSvcManager) Delete(name string) error {
	s.Stub.AddCall("Delete", name)

	if _, ok := MgrServices[name]; !ok {
		return c_ERROR_SERVICE_DOES_NOT_EXIST
	}
	delete(MgrServices, name)
	return nil
}

func (s *StubSvcManager) Create(name string, conf common.Conf) error {
	s.Stub.AddCall("Create", name, conf)

	if _, ok := MgrServices[name]; ok {
		return c_ERROR_SERVICE_EXISTS
	}

	MgrServices[name] = &service{
		running: false,
		conf:    conf,
	}
	return nil
}

func (s *StubSvcManager) Running(name string) (bool, error) {
	s.Stub.AddCall("Running", name)

	if svc, ok := MgrServices[name]; ok {
		return svc.running, nil
	}
	return false, c_ERROR_SERVICE_DOES_NOT_EXIST
}

func (s *StubSvcManager) Exists(name string, conf common.Conf) (bool, error) {
	if _, ok := MgrServices[name]; ok {
		return true, nil
	}
	return false, nil
}

// For now this doesn't do much since it doesn't help us test anything
// but we need it to implement the interface
func (s *StubSvcManager) ChangeServicePassword(name, newPassword string) error {
	s.Stub.AddCall("ChangeServicePassword", name, newPassword)

	if _, ok := MgrServices[name]; !ok {
		return c_ERROR_SERVICE_DOES_NOT_EXIST
	}

	return nil
}

func (s *StubSvcManager) ListServices() ([]string, error) {
	s.Stub.AddCall("listServices")

	services := []string{}
	for i := range MgrServices {
		services = append(services, i)
	}
	return services, s.NextErr()
}

func (s *StubSvcManager) Clear() {
	MgrServices = map[string]*service{}
}
