// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import (
	"context"

	charmscommon "github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
)

// Client provides an interface for interacting with the
// CAASFirewallerAPI. Subsets of this should be passed
// to the CAASFirewaller worker.
type Client interface {
	CAASFirewallerAPI
	LifeGetter
}

// CAASFirewallerAPI provides an interface for
// watching for the lifecycle state changes
// (including addition) of applications in the
// model, and fetching their details.
type CAASFirewallerAPI interface {
	WatchApplications(context.Context) (watcher.StringsWatcher, error)
	WatchApplication(context.Context, string) (watcher.NotifyWatcher, error)

	IsExposed(context.Context, string) (bool, error)
	ApplicationConfig(context.Context, string) (config.ConfigAttributes, error)

	ApplicationCharmInfo(ctx context.Context, appName string) (*charmscommon.CharmInfo, error)
}

// LifeGetter provides an interface for getting the
// lifecycle state value for an application.
type LifeGetter interface {
	Life(context.Context, string) (life.Value, error)
}

// PortService provides access to the port service.
type PortService interface {
	// WatchApplicationOpenedPorts returns a strings watcher for opened ports. This
	// watcher emits for changes to the opened ports table. Each emitted event contains
	// the app name which is associated with the changed port range.
	WatchApplicationOpenedPorts(ctx context.Context) (watcher.StringsWatcher, error)

	// GetApplicationOpenedPortsByEndpoint returns all the opened ports for the given
	// application, across all units, grouped by endpoint.
	//
	// NOTE: The returned port ranges are atomised, meaning we guarantee that each
	// port range is of unit length.
	GetApplicationOpenedPortsByEndpoint(context.Context, application.ID) (network.GroupedPortRanges, error)
}

// ApplicationService provides access to the application service.
type ApplicationService interface {
	// GetApplicationIDByName returns a application ID by application name. It
	// returns an error if the application can not be found by the name.
	GetApplicationIDByName(ctx context.Context, name string) (application.ID, error)
}
