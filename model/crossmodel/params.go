// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"gopkg.in/juju/charm.v6-unstable"
)

// OfferedServiceDetails represents a remote service used when vendor
// lists their own services.
type OfferedServiceDetails struct {
	// ServiceName is the service name.
	ServiceName string

	// ServiceURL is the URl where the service can be located.
	ServiceURL string

	// CharmName is a name of a charm for remote service.
	CharmName string

	// Endpoints are the charm endpoints supported by the service.
	// TODO(wallyworld) - do not use charm.Relation here
	Endpoints []charm.Relation

	// ConnectedCount are the number of users that are consuming the service.
	ConnectedCount int
}

// ListOffersResult is a result of listing a remote service.
type ListOffersResult struct {
	// Result contains remote service information.
	Result *OfferedServiceDetails

	// Error contains error related to this item.
	Error error
}
