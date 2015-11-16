// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/errors"

	"github.com/juju/juju/model/crossmodel"
)

// ServiceAPIFactory instances create ServiceDirectory instance.
type ServiceOffersAPIFactory interface {

	// ServiceOffers returns a service directory used to look up services
	// based on the specified directory name.
	ServiceOffers(directory string) (ServiceOffersAPI, error)
}

type createServiceDirectoryFunc func() crossmodel.ServiceDirectory

type serviceOffersAPIFactory struct {
	createLocalServiceDirectory createServiceDirectoryFunc
}

func newServiceAPIFactory(createFunc createServiceDirectoryFunc) (ServiceOffersAPIFactory, error) {
	return &serviceOffersAPIFactory{createFunc}, nil
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
