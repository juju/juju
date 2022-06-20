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
	Name() string
	Type() string
}

// ServiceLocatorState implements the ServiceLocatorBackend indirection
// over state.State.
type ServiceLocatorState struct {
	st *state.State
}

func (s ServiceLocatorState) ServiceLocator(slId string, slName string, slType string) (ServiceLocatorBackend, error) {

	sl, err := s.st.ServiceLocators().AddServiceLocator(state.AddServiceLocatorParams{
		ServiceLocatorUUID: slId,
		Name:               slName,
		Type:               slType,
	})
	return &serviceLocator{sl}, err
}

func (s ServiceLocatorState) Name() string {
	// TODO(anvial) TBW
	return ""
}

func (s ServiceLocatorState) Type() string {
	// TODO(anvial) TBW
	return ""
}

type ServiceLocatorAPI struct {
	backend ServiceLocatorBackend

	logger loggo.Logger
}

// NewServiceLocatorAPI returns a new ServiceLocatorAPI.
func NewServiceLocatorAPI(
	st *state.State,
	logger loggo.Logger,
) *ServiceLocatorAPI {
	return &ServiceLocatorAPI{
		ServiceLocatorState{st},
		logger,
	}
}

type serviceLocator struct {
	*state.ServiceLocator
}
