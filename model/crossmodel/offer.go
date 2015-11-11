// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// crossmodel is service layer package that supports cross model relations
// functionality.
package crossmodel

import (
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
)

// RemoteService has information about remote service.
type RemoteService struct {
	// Service has service's tag.
	Service names.ServiceTag

	// URL is Juju location where exported service's endpoints are.
	URL string

	// Description is description for the exported service.
	// For now, this defaults to description provided in the charm.
	// It could later be moved to describe a single exported endpoint.
	Description string

	// Users is the list of user tags that are given permission to this exported service.
	Users []names.UserTag
}

// Offer holds information about offered service and its endpoints.
type Offer struct {
	// RemoteService has information about offered service.
	RemoteService

	// Endpoints are service's endpoint names that are being offered.
	Endpoints []string
}

// RemoteEndpoint has information about remote service relation.
type RemoteEndpoint struct {
	// Name is a name of exported relation.
	Name string

	// Interface is an interface of exported relation.
	Interface string

	// Role is a role of exported relation.
	Role string
}

// RemoteServiceEndpoints has information about remote service and its
// exported endpoints.
type RemoteServiceEndpoints struct {
	// RemoteService has information about offered service.
	RemoteService

	// Endpoints are service's endpoint details that have been exported.
	Endpoints []RemoteEndpoint
}

//////////////////////TEMP PLACEHOLDER REMOVE WHEN REAL THING IS PLUGGED IN???????????
type ExporterStub struct{}

func (e ExporterStub) ExportOffer(offer Offer) error { return nil }

func (e ExporterStub) Search(filter params.EndpointsSearchFilter) ([]RemoteServiceEndpoints, error) {
	return nil, nil
}

//////////////////////END TEMP PLACEHOLDER REMOVE WHEN REAL THING IS PLUGGED IN???????????
