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
	AddServiceLocator(string, string, string) (*ServiceLocator, error)
	//AllServiceLocators() ([]*serviceLocator, error)
}

// ServiceLocatorState implements the ServiceLocatorBackend indirection
// over state.State.
type ServiceLocatorState struct {
	st *state.State
}

func (s ServiceLocatorState) AddServiceLocator(slId string, slName string, slType string) (*ServiceLocator, error) {
	sl, err := s.st.ServiceLocatorsState().AddServiceLocator(state.AddServiceLocatorParams{
		ServiceLocatorUUID: slId,
		Name:               slName,
		Type:               slType,
	})
	return &ServiceLocator{sl}, err
}

//func (s ServiceLocatorState) ServiceLocator(id string) (ServiceLocatorBackend, error) {
//	sl, err := s.st.ServiceLocatorsState.ServiceLocator(id)
//	return &lxdProfileMachine{m}, err
//}

//func (s ServiceLocatorState) AllServiceLocators() ([]*serviceLocator, error) {
//	sls, err := s.st.ServiceLocators().AllServiceLocators()
//	return sls, err
//}

//func (s ServiceLocatorState) Name() string {
//	return s.sl.Name()
//}
//
//func (s ServiceLocatorState) Type() string {
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
		ServiceLocatorState{st},
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

type ServiceLocator struct {
	*state.ServiceLocator
}
