// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/errors"

	"github.com/juju/juju/model/crossmodel"
)

// ServiceAPIFactory instances create ServiceDirectory instance.
type ServiceAPIFactory interface {

	// ServiceDirectoryForURL returns a service directory used to look up services
	// based on the specified URL.
	ServiceDirectoryForURL(url string) (ServiceOffersAPI, error)

	// ServiceDirectory returns a service directory used to look up services
	// based on the specified directory name.
	ServiceDirectory(directory string) (ServiceOffersAPI, error)
}

type createServiceDirectoryFunc func() crossmodel.ServiceDirectory

type serviceAPIFactory struct {
	createLocalServiceDirectory createServiceDirectoryFunc
}

func newServiceAPIFactory(createFunc createServiceDirectoryFunc) (ServiceAPIFactory, error) {
	return &serviceAPIFactory{createFunc}, nil
}

// ServiceDirectoryForURL returns a service directory used to look up services
// based on the specified URL.
func (s *serviceAPIFactory) ServiceDirectoryForURL(urlStr string) (ServiceOffersAPI, error) {
	url, err := crossmodel.ParseServiceURL(urlStr)
	if err != nil {
		return nil, err
	}
	return s.ServiceDirectory(url.Scheme)
}

// ServiceDirectory returns a service directory used to look up services
// based on the specified directory name.
func (s *serviceAPIFactory) ServiceDirectory(directory string) (ServiceOffersAPI, error) {
	switch directory {
	case "local":
		return &localServiceOffers{s.createLocalServiceDirectory()}, nil
	}
	return nil, errors.NotSupportedf("service directory for %q", directory)
}
