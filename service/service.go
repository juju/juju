// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/errors"
	"github.com/juju/names"
)

type services interface {
	InitSystem() string
	Start(name string) error
	Stop(name string) error
	IsRunning(name string) (bool, error)
	Enable(name string) error
	Disable(name string) error
	IsEnabled(name string) (bool, error)
	Manage(name string, conf Conf) error
	Remove(name string) error
	Install(name string, conf Conf) error
	Check(name string, conf Conf) (bool, error)
	IsManaged(name string) bool
}

// Service is a convenience wrapper around Services for a single service.
type Service struct {
	name     string
	conf     Conf
	services services
}

// NewService is a bare-bones "constructor" for a new Service.
func NewService(name string, conf Conf, services services) *Service {
	return &Service{
		name:     name,
		conf:     conf,
		services: services,
	}
}

// DiscoverService builds a Services value using the provided dataDir and
// init system name and wraps it in a Service for the given name and
// conf.
func DiscoverService(name, dataDir string, conf Conf, args ...string) (*Service, error) {
	services, err := DiscoverServices(dataDir, args...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	svc := NewService(name, conf, services)
	return svc, nil
}

// NewAgentService builds a new Service for the juju agent identified
// by the provided information and returns it.
func NewAgentService(tag names.Tag, paths AgentPaths, env map[string]string, services services) (*Service, error) {
	spec, err := newAgentServiceSpec(tag, paths, env)
	if err != nil {
		return nil, errors.Trace(err)
	}
	spec.initSystem = services.InitSystem()

	svc := NewService(spec.Name(), spec.Conf(), services)
	return svc, nil
}

// Name returns the name of the service.
func (s Service) Name() string {
	return s.name
}

// Conf returns a copy of the service's conf specification..
func (s Service) Conf() Conf {
	return s.conf
}

// Start starts the service.
func (s Service) Start() error {
	return s.services.Start(s.name)
}

// Stop stops the service.
func (s Service) Stop() error {
	return s.services.Stop(s.name)
}

// IsRunning returns true if the service is running.
func (s Service) IsRunning() (bool, error) {
	return s.services.IsRunning(s.name)
}

// Enable registers the service with the underlying init system.
func (s Service) Enable() error {
	return s.services.Enable(s.name)
}

// Disable unregisters the service with the underlying init system.
func (s Service) Disable() error {
	return s.services.Disable(s.name)
}

// IsEnabled returns true if the init system knows about the service.
func (s Service) IsEnabled() (bool, error) {
	return s.services.IsEnabled(s.name)
}

// Manage adds the service to the set of services that juju manages.
func (s Service) Manage() error {
	return s.services.Manage(s.name, s.conf)
}

// Remove disables the service and removes it from juju management.
func (s Service) Remove() error {
	return s.services.Remove(s.name)
}

// Install adds the service to juju management, then enables and starts it.
func (s Service) Install() error {
	return s.services.Install(s.name, s.conf)
}

// Check returns true if the service is managed by juju and also matches
// the service in the init system with the same name (if any exists).
func (s Service) Check() (bool, error) {
	return s.services.Check(s.name, s.conf)
}

// IsManaged returns true if the service is currently managed by juju.
func (s Service) IsManaged() bool {
	return s.services.IsManaged(s.name)
}
