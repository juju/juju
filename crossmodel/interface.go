// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import "github.com/juju/names"

// BaseDetails holds information about offered service and its endpoints.
type BaseDetails interface {
	// Service has service's tag.
	Service() names.ServiceTag

	// Endpoints list of service's endpoints that are being offered.
	Endpoints() []string
}

// Offer holds information about service's offer.
type Offer interface {
	BaseDetails

	// URL is the location where these endpoints will be accessible from.
	URL() string

	// Users is the list of user tags that are given permission to these endpoints.
	Users() []names.UserTag
}

// ServiceDetails holds additional information about offered service and its endpoints.
type ServiceDetails interface {
	BaseDetails

	// Description is description of this service.
	Description() string
}
