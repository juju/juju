// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner

import (
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/watcher"
)

// Client provides an interface for interacting with the
// CAASUnitProvisioner API. Subsets of this should be passed
// to the CAASUnitProvisioner worker.
type Client interface {
	ApplicationGetter
	ContainerSpecGetter
	LifeGetter
	UnitGetter
}

// ApplicationGetter provides an interface for
// watching for the lifecycle state changes
// (including addition) of applications in the
// model, and fetching their details.
type ApplicationGetter interface {
	WatchApplications() (watcher.StringsWatcher, error)
}

// ContainerSpecGetter provides an interface for
// watching and getting the container spec for a
// unit.
type ContainerSpecGetter interface {
	ContainerSpec(entityName string) (string, error)
	WatchContainerSpec(entityName string) (watcher.NotifyWatcher, error)
}

// LifeGetter provides an interface for getting the
// lifecycle state value for an application or unit.
type LifeGetter interface {
	Life(string) (life.Value, error)
}

// UnitGetter provides an interface for watching for
// the lifecycle state changes (including addition)
// of a specified application's units, and fetching
// their details.
type UnitGetter interface {
	WatchUnits(string) (watcher.StringsWatcher, error)
}
