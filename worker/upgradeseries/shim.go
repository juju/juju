// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package upgradeseries

import (
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/upgradeseries"
	"github.com/juju/juju/watcher"
)

// Facade exposes the API surface required by the upgrade-series worker.
//go:generate mockgen -package upgradeseries_test -destination facade_mock_test.go github.com/juju/juju/worker/upgradeseries Facade
type Facade interface {
	WatchUpgradeSeriesNotifications() (watcher.NotifyWatcher, error)
}

// NewFacade creates a *upgradeseries.State and returns it as a Facade.
func NewFacade(apiCaller base.APICaller, tag names.Tag) Facade {
	return upgradeseries.NewState(apiCaller, tag)
}
