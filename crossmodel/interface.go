// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import "github.com/juju/juju/apiserver/params"

// Exporter provides methods to export and inspect services endpoints.
type Exporter interface {
	// ExportOffer prepares service endpoints for consumption.
	// An actual implementation will coordinate the work:
	// validate entities exist, access the service directory, write to state etc.
	ExportOffer(offer Offer) error

	// Search looks through offered services and returns the ones
	// that match specified filter.
	Search(filter params.EndpointsSearchFilter) ([]RemoteServiceEndpoints, error)
}
