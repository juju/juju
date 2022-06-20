// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/loggo"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/state"
)

// ServiceLocatorBackend describes service locator state methods
// for executing a service locator upgrade.
type ServiceLocatorBackend interface {
	Id() (string, error)
	Name() string
	Tag() string
}

// ServiceLocatorState implements the ServiceLocatorBackend indirection
// over state.State.
type ServiceLocatorState struct {
	st *state.State
}

func (s LXDProfileStateV2) ServiceLocator(name string) (ServiceLocatorBackend, error) {
	sl, err := s.st.ServiceLocator(name)
	return &serviceLocator{sl}, err
}

type ServiceLocatorAPI struct {
	backend   ServiceLocatorBackend
	resources facade.Resources

	logger loggo.Logger
}

// NewServiceLocatorAPI returns a new ServiceLocatorAPI.
func NewServiceLocatorAPI(
	backend ServiceLocatorBackend,
	resources facade.Resources,
	authorizer facade.Authorizer,
	logger loggo.Logger,
) *ServiceLocatorAPI {
	logger.Tracef("ServiceLocatorAPI called with %s", authorizer.GetAuthTag())
	return &ServiceLocatorAPI{
		backend:   backend,
		resources: resources,
		logger:    logger,
	}
}

type serviceLocator struct {
	*state.ServiceLocator
}
