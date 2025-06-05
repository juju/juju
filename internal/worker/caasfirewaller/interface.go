// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import (
	"context"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/application/charm"
	internalcharm "github.com/juju/juju/internal/charm"
)

// PortService provides access to the port service.
type PortService interface {
	// WatchOpenedPortsForApplication returns a notify watcher for opened ports. This
	// watcher emits events for changes to the opened ports table that are associated
	// with the given application
	WatchOpenedPortsForApplication(context.Context, application.ID) (watcher.NotifyWatcher, error)

	// GetApplicationOpenedPortsByEndpoint returns all the opened ports for the given
	// application, across all units, grouped by endpoint.
	//
	// NOTE: The returned port ranges are atomised, meaning we guarantee that each
	// port range is of unit length.
	GetApplicationOpenedPortsByEndpoint(context.Context, application.ID) (network.GroupedPortRanges, error)
}

// ApplicationService provides access to the application service.
type ApplicationService interface {
	// GetApplicationName returns the name of the specified application.
	// The following errors may be returned:
	// - [applicationerrors.ApplicationNotFound] if the application does not exist
	GetApplicationName(context.Context, application.ID) (string, error)

	// GetApplicationLifelooks up the life of the specified application, returning
	// an error satisfying [applicationerrors.ApplicationNotFoundError] if the
	// application is not found.
	GetApplicationLife(context.Context, application.ID) (life.Value, error)

	// IsApplicationExposed returns whether the provided application is exposed or not.
	//
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	IsApplicationExposed(ctx context.Context, name string) (bool, error)

	// GetCharmByApplicationID returns the charm for the specified application
	// ID.
	//
	// If the application does not exist, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned. If the charm for the
	// application does not exist, an error satisfying
	// [applicationerrors.CharmNotFound is returned. If the application name is not
	// valid, an error satisfying [applicationerrors.ApplicationNameNotValid] is
	// returned.
	GetCharmByApplicationID(context.Context, application.ID) (internalcharm.Charm, charm.CharmLocator, error)

	// WatchApplicationExposed watches for changes to the specified application's
	// exposed endpoints.
	// This notifies on any changes to the application's exposed endpoints. It is up
	// to the caller to determine if the exposed endpoints they're interested in has
	// changed.
	//
	// If the application does not exist an error satisfying
	// [applicationerrors.ApplicationNotFound] will be returned.
	WatchApplicationExposed(ctx context.Context, name string) (watcher.NotifyWatcher, error)

	// WatchApplications returns a watcher that emits application uuids when
	// applications are added or removed.
	WatchApplications(context.Context) (watcher.StringsWatcher, error)
}
