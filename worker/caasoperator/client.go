// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import (
	"github.com/juju/version"
	"gopkg.in/juju/charm.v6"

	caasoperatorapi "github.com/juju/juju/api/caasoperator"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
)

// Client provides an interface for interacting
// with the CAASOperator API. Subsets of this
// should be passed to the CAASOperator worker.
type Client interface {
	CharmGetter
	UnitGetter
	UnitRemover
	ApplicationWatcher
	ContainerStartWatcher
	StatusSetter
	VersionSetter
	Model() (*model.Model, error)
}

// CharmGetter provides an interface for getting
// the URL and SHA256 hash of the charm currently
// assigned to the application.
type CharmGetter interface {
	Charm(application string) (*caasoperatorapi.CharmInfo, error)
}

// UnitGetter provides an interface for watching for
// the lifecycle state changes (including addition)
// of a specified application's units, and fetching
// their details.
type UnitGetter interface {
	WatchUnits(string) (watcher.StringsWatcher, error)
	Life(string) (life.Value, error)
}

// UnitRemover provides an interface for
// removing a unit.
type UnitRemover interface {
	RemoveUnit(string) error
}

// ApplicationWatcher provides an interface watching
// for application changes.
type ApplicationWatcher interface {
	Watch(string) (watcher.NotifyWatcher, error)
}

// ContainerStartWatcher provides an interface for watching
// for unit starts.
type ContainerStartWatcher interface {
	WatchContainerStart(string, string) (watcher.StringsWatcher, error)
}

// StatusSetter provides an interface for setting
// the status of a CAAS application.
type StatusSetter interface {
	// SetStatus sets the status of an application.
	SetStatus(
		application string,
		status status.Status,
		info string,
		data map[string]interface{},
	) error
}

// CharmConfigGetter provides an interface for
// watching and getting the application's charm config settings.
type CharmConfigGetter interface {
	CharmConfig(string) (charm.Settings, error)
	WatchCharmConfig(string) (watcher.NotifyWatcher, error)
}

// VersionSetter provides an interface for setting
// the operator agent version.
type VersionSetter interface {
	SetVersion(appName string, v version.Binary) error
}
