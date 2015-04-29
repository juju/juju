// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.
//
// +build !windows

package windows

import (
	"github.com/juju/juju/service/common"
)

type SvcManager struct{}

func (s *SvcManager) Start(name string) error {
	return nil
}

func (s *SvcManager) Stop(name string) error {
	return nil
}

func (s *SvcManager) Delete(name string) error {
	return nil
}

func (s *SvcManager) Create(name string, conf common.Conf) error {
	return nil
}

func (s *SvcManager) Running(name string) (bool, error) {
	return false, nil
}

func (s *SvcManager) Exists(name string, conf common.Conf) (bool, error) {
	return false, nil
}

var listServices = func() ([]string, error) {
	return []string{}, nil
}

var newServiceManager = func() (ServiceManagerInterface, error) {
	return &SvcManager{}, nil
}
