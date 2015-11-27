// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"gopkg.in/juju/charm.v6-unstable"
)

// ListEndpointsService represents a remote service used when vendor
// lists their own services.
type ListEndpointsService struct {
	// ServiceName is the service name.
	ServiceName string

	// ServiceURL is the URl where the service can be located.
	ServiceURL string

	// CharmName is a name of a charm for remote service.
	CharmName string

	// Endpoints are the charm endpoints supported by the service.
	Endpoints []charm.Relation

	// ConnectedCount are the number of users that are consuming the service.
	ConnectedCount int
}

// ListEndpointsServiceResult is a result of listing a remote service.
type ListEndpointsServiceResult struct {
	// Result contains remote service information.
	Result *ListEndpointsService

	// Error contains error related to this item.
	Error error
}

// AddRelationResults holds the results of a AddRelation call. The Endpoints
// field maps service names to the involved endpoints.
type AddRelationResults struct {
	Endpoints map[string]charm.Relation
}
