// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries

import (
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/agent/upgradeseries"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
)

// Facade exposes the API surface required by the upgrade-series worker.
type Facade interface {
	// Getters
	WatchUpgradeSeriesNotifications() (watcher.NotifyWatcher, error)
	MachineStatus() (model.UpgradeSeriesStatus, error)
	UnitsPrepared() ([]names.UnitTag, error)
	UnitsCompleted() ([]names.UnitTag, error)
	CurrentSeries() (string, error)
	TargetSeries() (string, error)

	// Setters
	StartUnitCompletion(reason string) error
	SetMachineStatus(status model.UpgradeSeriesStatus, reason string) error
	FinishUpgradeSeries(string) error
	PinMachineApplications() (map[string]error, error)
	UnpinMachineApplications() (map[string]error, error)
	SetInstanceStatus(model.UpgradeSeriesStatus, string) error
}

// NewFacade creates a new upgrade-series client and returns its
// reference as the facade indirection above.
func NewFacade(apiCaller base.APICaller, tag names.Tag) Facade {
	return upgradeseries.NewClient(apiCaller, tag)
}
