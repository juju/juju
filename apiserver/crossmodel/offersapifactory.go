// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/state"
)

// ApplicationOffersAPIFactory instances create ApplicationDirectory instance.
type ApplicationOffersAPIFactory interface {
	facade.Resource

	// ApplicationOffers returns a application directory used to look up services
	// based on the specified directory name.
	ApplicationOffers(directory string) (ApplicationOffersAPI, error)
}

type createApplicationDirectoryFunc func() crossmodel.ApplicationDirectory

type closer interface {
	Close() error
}

type applicationOffersAPIFactory struct {
	createLocalApplicationDirectory createApplicationDirectoryFunc
	closer                          closer
}

// newServiceAPIFactory creates a ApplicationOffersAPIFactory instance which
// is used to provide access to services for specified directories eg "local" or "vendor".
func newServiceAPIFactory(createFunc createApplicationDirectoryFunc, closer closer) (ApplicationOffersAPIFactory, error) {
	return &applicationOffersAPIFactory{createFunc, closer}, nil
}

// ApplicationOffers returns a application directory used to look up services
// based on the specified directory name.
func (s *applicationOffersAPIFactory) ApplicationOffers(directory string) (ApplicationOffersAPI, error) {
	switch directory {
	case "local":
		return &localApplicationOffers{s.createLocalApplicationDirectory()}, nil
	}
	return nil, errors.NotSupportedf("application directory for %q", directory)
}

// Stop is required by the Resource interface.
func (s *applicationOffersAPIFactory) Stop() error {
	if s.closer != nil {
		return s.closer.Close()
	}
	return nil
}

// ApplicationOffersAPIFactoryResource is a function which returns the application offer api factory
// which can be used as an facade resource.
func ApplicationOffersAPIFactoryResource(st *state.State) (facade.Resource, error) {
	controllerModelState := st
	controllerModel, err := st.ControllerModel()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var closer closer
	if st.ModelTag() != controllerModel.ModelTag() {
		// We are not using the state server environment, so get one.
		logger.Debugf("getting a controller connection, current model: %s", st.ModelTag())
		controllerModelState, err = st.ForModel(controllerModel.ModelTag())
		if err != nil {
			return nil, errors.Trace(err)
		}
		closer = controllerModelState
	}
	return newServiceAPIFactory(
		func() crossmodel.ApplicationDirectory {
			return state.NewApplicationDirectory(controllerModelState)
		}, closer)
}
