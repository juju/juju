// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"gopkg.in/juju/charm.v6-unstable"
)

// OfferedApplicationDetails represents a remote application used when vendor
// lists their own applications.
type OfferedApplicationDetails struct {
	// ApplicationName is the application name.
	ApplicationName string

	// ApplicationURL is the URl where the application can be located.
	ApplicationURL string

	// CharmName is a name of a charm for remote application.
	CharmName string

	// Endpoints are the charm endpoints supported by the application.
	// TODO(wallyworld) - do not use charm.Relation here
	Endpoints []charm.Relation

	// ConnectedCount are the number of users that are consuming the application.
	ConnectedCount int
}

// OfferedApplicationDetailsResult is a result of listing a remote application.
type OfferedApplicationDetailsResult struct {
	// Result contains remote application information.
	Result *OfferedApplicationDetails

	// Error contains error related to this item.
	Error error
}
