// Copyright 2012 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/errors"

	"github.com/juju/juju/service/common"
)

// Service is a convenience wrapper around Services for a single service.
type Service struct {
	name     string
	conf     common.Conf
	services *Services
}

func NewService(name, dataDir string, conf common.Conf, args ...string) (*Service, error) {
	services, err := NewServices(dataDir, args...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	svc := newService(name, conf, services)
	return svc, nil
}

func newService(name string, conf common.Conf, services *Services) *Service {
	return &Service{
		name:     name,
		conf:     conf,
		services: services,
	}
}

func (s Service) Name() string {
	return s.name
}

func (s Service) Conf() common.Conf {
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

func (s Service) Add() error {
	return s.services.Add(s.name, &s.conf)
}

func (s Service) Remove() error {
	return s.services.Remove(s.name)
}

func (s Service) Check() (bool, error) {
	return s.services.Check(s.name, &s.conf)
}

func (s Service) IsManaged() bool {
	return s.services.IsManaged(s.name)
}
