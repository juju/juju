// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import (
	"context"

	charmscommon "github.com/juju/juju/api/common/charms"
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
	WatchOpenedPorts(ctx context.Context) (watcher.StringsWatcher, error)
	GetOpenedPorts(ctx context.Context, appName string) (network.GroupedPortRanges, error)

	IsExposed(context.Context, string) (bool, error)
	ApplicationConfig(context.Context, string) (config.ConfigAttributes, error)

	ApplicationCharmInfo(ctx context.Context, appName string) (*charmscommon.CharmInfo, error)
}

// LifeGetter provides an interface for getting the
// lifecycle state value for an application.
type LifeGetter interface {
	Life(context.Context, string) (life.Value, error)
}
