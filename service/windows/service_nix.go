// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.
//
// +build !windows

package windows

import (
	"github.com/juju/juju/service/common"
)

// SvcManager implements ServiceManager interface
type SvcManager struct{}

// Start starts a service.
func (s *SvcManager) Start(name string) error {
	return nil
}

// Stop stops a service.
func (s *SvcManager) Stop(name string) error {
	return nil
}

// Delete deletes a service.
func (s *SvcManager) Delete(name string) error {
	return nil
}

// Create creates a service with the given config.
func (s *SvcManager) Create(name string, conf common.Conf) error {
	return nil
}

// Running returns the status of a service.
func (s *SvcManager) Running(name string) (bool, error) {
	return false, nil
}

// Exists checks whether the config of the installed service matches the
// config supplied to this function
func (s *SvcManager) Exists(name string, conf common.Conf) (bool, error) {
	return false, nil
}

// ChangeServicePassword can change the password of a service
// as long as it belongs to the user defined in this package
func (s *SvcManager) ChangeServicePassword(name, newPassword string) error {
	return nil
}

var listServices = func() ([]string, error) {
	return []string{}, nil
}

var NewServiceManager = func() (ServiceManager, error) {
	return &SvcManager{}, nil
}
