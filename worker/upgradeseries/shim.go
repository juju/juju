// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries

import (
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/upgradeseries"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/watcher"
)

// Facade exposes the API surface required by the upgrade-series worker.
type Facade interface {
	WatchUpgradeSeriesNotifications() (watcher.NotifyWatcher, error)
	UpgradeSeriesStatus(model.UpgradeSeriesStatusType) ([]string, error)
	CompleteUnitUpgradeSeries() error
	MachineStatus() (model.UpgradeSeriesStatus, error)
	SetMachineStatus(status model.UpgradeSeriesStatus) error
}

// NewFacade creates a *upgradeseries.Client and returns it as a Facade.
func NewFacade(apiCaller base.APICaller, tag names.Tag) Facade {
	return upgradeseries.NewState(apiCaller, tag)
}
