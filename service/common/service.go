// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
)

// Service is the base type for service.Service implementations.
type Service struct {
	// Name is the name of the service.
	Name string

	// Conf holds the info used to build an init system conf.
	Conf Conf
}

// NoConf checks whether or not Conf has been set.
func (s Service) NoConf() bool {
	return s.Conf.IsZero()
}

// Validate checks the service's values for correctness.
func (s Service) Validate(os string) error {
	if s.Name == "" {
		return errors.New("missing Name")
	}

	if err := s.Conf.Validate(os); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// UpdateConfig implements service.Service.
func (s *Service) UpdateConfig(conf Conf) {
	s.Conf = conf
}
