// Copyright 2012 Canonical Ltd.
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

func NewService(name, dataDir string, conf Conf, args ...string) (*Service, error) {
	services, err := DiscoverServices(dataDir, args...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	svc := WrapService(name, conf, services)
	return svc, nil
}

func WrapService(name string, conf Conf, services services) *Service {
	return &Service{
		name:     name,
		conf:     conf,
		services: services,
	}
}

func WrapAgentService(tag names.Tag, paths AgentPaths, env map[string]string, services services) (*Service, error) {
	spec, err := newAgentService(tag, paths, env)
	if err != nil {
		return nil, errors.Trace(err)
	}
	spec.initSystem = services.InitSystem()

	svc := WrapService(spec.Name(), spec.Conf(), services)
	return svc, nil
}

func (s Service) Name() string {
	return s.name
}

func (s Service) Conf() Conf {
	return s.conf
}

func (s Service) Start() error {
	return s.services.Start(s.name)
}

func (s Service) Stop() error {
	return s.services.Stop(s.name)
}

func (s Service) IsRunning() (bool, error) {
	return s.services.IsRunning(s.name)
}

func (s Service) Enable() error {
	return s.services.Enable(s.name)
}

func (s Service) Disable() error {
	return s.services.Disable(s.name)
}

func (s Service) IsEnabled() (bool, error) {
	return s.services.IsEnabled(s.name)
}

func (s Service) Manage() error {
	return s.services.Manage(s.name, s.conf)
}

func (s Service) Remove() error {
	return s.services.Remove(s.name)
}

func (s Service) Install() error {
	return s.services.Install(s.name, s.conf)
}

func (s Service) Check() (bool, error) {
	return s.services.Check(s.name, s.conf)
}

func (s Service) IsManaged() bool {
	return s.services.IsManaged(s.name)
}
