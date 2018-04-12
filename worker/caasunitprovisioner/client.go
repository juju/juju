// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner

import (
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/watcher"
)

// Client provides an interface for interacting with the
// CAASUnitProvisioner API. Subsets of this should be passed
// to the CAASUnitProvisioner worker.
type Client interface {
	ApplicationGetter
	ApplicationUpdater
	PodSpecGetter
	LifeGetter
	UnitGetter
	UnitUpdater
}

// ApplicationGetter provides an interface for
// watching for the lifecycle state changes
// (including addition) of applications in the
// model, and fetching their details.
type ApplicationGetter interface {
	WatchApplications() (watcher.StringsWatcher, error)
	ApplicationConfig(string) (application.ConfigAttributes, error)
}

// ApplicationUpdater provides an interface for updating
// Juju applications from changes in the cloud.
type ApplicationUpdater interface {
	UpdateApplicationService(arg params.UpdateApplicationServiceArg) error
}

// PodSpecGetter provides an interface for
// watching and getting the pod spec for an application.
type PodSpecGetter interface {
	PodSpec(appName string) (string, error)
	WatchPodSpec(appName string) (watcher.NotifyWatcher, error)
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

// UnitUpdater provides an interface for updating
// Juju units from changes in the cloud.
type UnitUpdater interface {
	UpdateUnits(arg params.UpdateApplicationUnits) error
}
