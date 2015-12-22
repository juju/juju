// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/model/crossmodel"
	"github.com/juju/juju/state"
)

// ServiceOffersAPIFactory instances create ServiceDirectory instance.
type ServiceOffersAPIFactory interface {
	common.Resource

	// ServiceOffers returns a service directory used to look up services
	// based on the specified directory name.
	ServiceOffers(directory string) (ServiceOffersAPI, error)
}

type createServiceDirectoryFunc func() crossmodel.ServiceDirectory

type closer interface {
	Close() error
}

type serviceOffersAPIFactory struct {
	createLocalServiceDirectory createServiceDirectoryFunc
	closer                      closer
}

// newServiceAPIFactory creates a ServiceOffersAPIFactory instance which
// is used to provide access to services for specified directories eg "local" or "vendor".
func newServiceAPIFactory(createFunc createServiceDirectoryFunc, closer closer) (ServiceOffersAPIFactory, error) {
	return &serviceOffersAPIFactory{createFunc, closer}, nil
}

// ServiceOffers returns a service directory used to look up services
// based on the specified directory name.
func (s *serviceOffersAPIFactory) ServiceOffers(directory string) (ServiceOffersAPI, error) {
	switch directory {
	case "local":
		return &localServiceOffers{s.createLocalServiceDirectory()}, nil
	}
	return nil, errors.NotSupportedf("service directory for %q", directory)
}

// Stop is required by the Resource interface.
func (s *serviceOffersAPIFactory) Stop() error {
	if s.closer != nil {
		return s.closer.Close()
	}
	return nil
}

// ServiceOffersAPIFactoryResource is a function which returns the service offer api factory
// which can be used as an facade resource.
func ServiceOffersAPIFactoryResource(st *state.State) (common.Resource, error) {
	ssState := st
	env, err := st.StateServerEnvironment()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var closer closer
	if st.EnvironTag() != env.EnvironTag() {
		// We are not using the state server environment, so get one.
		logger.Debugf("getting a state server state connection, current env: %s", st.EnvironTag())
		ssState, err = st.ForEnviron(env.EnvironTag())
		if err != nil {
			return nil, errors.Trace(err)
		}
		closer = ssState
	}
	return newServiceAPIFactory(
		func() crossmodel.ServiceDirectory {
			return state.NewServiceDirectory(ssState)
		}, closer)
}
