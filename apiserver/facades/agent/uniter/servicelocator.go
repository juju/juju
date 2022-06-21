// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/loggo"

	"github.com/juju/juju/state"
)

// ServiceLocatorBackend describes service locator state methods
// for executing a service locator upgrade.
type ServiceLocatorBackend interface {
	ServiceLocator(string, string, string) (*serviceLocator, error)
	//AllServiceLocators() ([]*serviceLocator, error)
}

// ServiceLocatorBase implements the ServiceLocatorBackend indirection
// over state.State.
type ServiceLocatorBase struct {
	st *state.State
}

func (s ServiceLocatorBase) ServiceLocator(slId string, slName string, slType string) (*serviceLocator, error) {
	sl, err := s.st.ServiceLocators().AddServiceLocator(state.AddServiceLocatorParams{
		ServiceLocatorUUID: slId,
		Name:               slName,
		Type:               slType,
	})
	return &serviceLocator{sl}, err
}

//func (s ServiceLocatorBase) AllServiceLocators() ([]*serviceLocator, error) {
//	sls, err := s.st.ServiceLocators().AllServiceLocators()
//	return sls, err
//}

//func (s ServiceLocatorBase) Name() string {
//	return s.sl.Name()
//}
//
//func (s ServiceLocatorBase) Type() string {
//	return s.sl.Type()
//}

type ServiceLocatorAPI struct {
	backend ServiceLocatorBackend

	logger loggo.Logger
}

// NewExternalServiceLocatorAPI can be used for API registration.
func NewExternalServiceLocatorAPI(
	st *state.State,
	logger loggo.Logger,
) *ServiceLocatorAPI {
	return NewServiceLocatorAPI(
		ServiceLocatorBase{st},
		logger,
	)
}

// NewServiceLocatorAPI returns a new NewServiceLocatorAPI.
func NewServiceLocatorAPI(
	backend ServiceLocatorBackend,
	logger loggo.Logger,
) *ServiceLocatorAPI {
	return &ServiceLocatorAPI{
		backend: backend,
		logger:  logger,
	}
}

type serviceLocator struct {
	*state.ServiceLocator
}
