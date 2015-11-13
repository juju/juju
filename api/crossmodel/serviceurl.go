// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/model/crossmodel"
)

// ServiceAPIFactory instances create ServiceDirectory instance.
type ServiceAPIFactory interface {

	// ServiceDirectory returns the correct service directory to be
	// able to lookup service offers at the specified URL.
	ServiceDirectory(url string) (ServiceDirectoryAPI, error)
}

type serviceAPIFactory struct {
	st base.APICallCloser
}

// NewServiceAPIFactory returns a ServiceAPIFactory that talks to a Juju local controller.
func NewServiceAPIFactory(st base.APICallCloser) (ServiceAPIFactory, error) {
	return &serviceAPIFactory{st}, nil
}

// ServiceDirectory returns a service directory used to look up services
// hosted by a local Juju controller.
func (s *serviceAPIFactory) ServiceDirectory(urlStr string) (ServiceDirectoryAPI, error) {
	url, err := crossmodel.ParseServiceURL(urlStr)
	if err != nil {
		return nil, err
	}
	switch url.Scheme {
	case "local":
		return NewServiceDirectory(s.st), nil
	}
	return nil, errors.NotSupportedf("service URL with scheme %q", url.Scheme)
}
