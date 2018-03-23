// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import (
	"gopkg.in/juju/charm.v6"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/status"
	"github.com/juju/juju/watcher"
)

// Client provides an interface for interacting
// with the CAASOperator API. Subsets of this
// should be passed to the CAASOperator worker.
type Client interface {
	CharmGetter
	UnitGetter
	ApplicationWatcher
	PodSpecSetter
	StatusSetter
	ModelName() (string, error)
}

// CharmGetter provides an interface for getting
// the URL and SHA256 hash of the charm currently
// assigned to the application.
type CharmGetter interface {
	Charm(application string) (_ *charm.URL, force bool, sha256 string, vers int, _ error)
}

// UnitGetter provides an interface for watching for
// the lifecycle state changes (including addition)
// of a specified application's units, and fetching
// their details.
type UnitGetter interface {
	WatchUnits(string) (watcher.StringsWatcher, error)
	Life(string) (life.Value, error)
}

// ApplicationWatcher provides an interface for watching
// application changes.
type ApplicationWatcher interface {
	Watch(string) (watcher.NotifyWatcher, error)
}

// PodSpecSetter provides an interface for
// setting the pod spec for the application.
type PodSpecSetter interface {
	SetPodSpec(appName, spec string) error
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
