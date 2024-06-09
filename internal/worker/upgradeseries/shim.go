// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries

import (
	"context"

	"github.com/juju/names/v5"

	"github.com/juju/juju/api/agent/upgradeseries"
	"github.com/juju/juju/api/base"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
)

// Facade exposes the API surface required by the upgrade-series worker.
type Facade interface {
	// Getters
	WatchUpgradeSeriesNotifications(context.Context) (watcher.NotifyWatcher, error)
	MachineStatus() (model.UpgradeSeriesStatus, error)
	UnitsPrepared() ([]names.UnitTag, error)
	UnitsCompleted() ([]names.UnitTag, error)

	// Setters
	StartUnitCompletion(reason string) error
	SetMachineStatus(status model.UpgradeSeriesStatus, reason string) error
	FinishUpgradeSeries(corebase.Base) error
	PinMachineApplications(context.Context) (map[string]error, error)
	UnpinMachineApplications(context.Context) (map[string]error, error)
	SetInstanceStatus(model.UpgradeSeriesStatus, string) error
}

// NewFacade creates a new upgrade-series client and returns its
// reference as the facade indirection above.
func NewFacade(apiCaller base.APICaller, tag names.Tag) Facade {
	return upgradeseries.NewClient(apiCaller, tag)
}
