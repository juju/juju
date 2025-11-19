// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import (
	"context"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
)

// PortService provides access to the port service.
type PortService interface {
	// WatchOpenedPortsForApplication returns a notify watcher for opened ports. This
	// watcher emits events for changes to the opened ports table that are associated
	// with the given application
	WatchOpenedPortsForApplication(context.Context, application.UUID) (watcher.NotifyWatcher, error)

	// GetApplicationOpenedPortsByEndpoint returns all the opened ports for the given
	// application, across all units, grouped by endpoint.
	//
	// NOTE: The returned port ranges are atomised, meaning we guarantee that each
	// port range is of unit length.
	GetApplicationOpenedPortsByEndpoint(context.Context, application.UUID) (network.GroupedPortRanges, error)
}

// ApplicationService provides access to the application service.
type ApplicationService interface {
	// GetApplicationName returns the name of the specified application.
	// The following errors may be returned:
	// - [applicationerrors.ApplicationNotFound] if the application does not exist
	GetApplicationName(context.Context, application.UUID) (string, error)

	// GetApplicationLifelooks up the life of the specified application, returning
	// an error satisfying [applicationerrors.ApplicationNotFoundError] if the
	// application is not found.
	GetApplicationLife(context.Context, application.UUID) (life.Value, error)

	// WatchApplications returns a watcher that emits application uuids when
	// applications are added or removed.
	WatchApplications(context.Context) (watcher.StringsWatcher, error)
}
