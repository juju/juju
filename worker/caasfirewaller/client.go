// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import (
	"github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher"
)

// Client provides an interface for interacting with the
// CAASFirewaller API. Subsets of this should be passed
// to the CAASFirewaller worker.
type Client interface {
	ApplicationGetter
	LifeGetter
	CharmGetter
}

// ApplicationGetter provides an interface for
// watching for the lifecycle state changes
// (including addition) of applications in the
// model, and fetching their details.
type ApplicationGetter interface {
	WatchApplications() (watcher.StringsWatcher, error)
	WatchApplication(string) (watcher.NotifyWatcher, error)
	IsExposed(string) (bool, error)
	ApplicationConfig(string) (application.ConfigAttributes, error)
}

// LifeGetter provides an interface for getting the
// lifecycle state value for an application.
type LifeGetter interface {
	Life(string) (life.Value, error)
}

type CharmGetter interface {
	ApplicationCharmInfo(string) (*charms.CharmInfo, error)
}
