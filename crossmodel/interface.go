// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import "github.com/juju/names"

// HostingURL defines the location which identifies offered services endpoints..
type HostingURL struct {
	Protocol string
	User     string
	Env      string
	Service  string
}

// Offer holds information about service's offer.
type Offer struct {
	// Service has service's tag.
	Service names.ServiceTag

	// Endpoints list of service's endpoints that are being offered.
	Endpoints []string

	// URL is the location where these endpoitns will be accessible from.
	// TODO (anastasiamac 2015-11-03) change this to HostingURL type once it becomes
	// established and can be obtained from parsing a string.
	URL string

	// Users is the list of user tags that are given permission to these endpoints.
	Users []names.UserTag
}
