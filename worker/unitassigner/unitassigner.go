// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitassigner

import (
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/worker"
)

type UnitAssigner interface {
	AssignUnits() error
	WatchUnitAssignments() (watcher.NotifyWatcher, error)
}

func New(ua UnitAssigner) worker.Worker {
	return worker.NewNotifyWorker(unitAssigner{api: ua})
}

type unitAssigner struct {
	api UnitAssigner
}

func (u unitAssigner) SetUp() (watcher.NotifyWatcher, error) {
	return u.api.WatchUnitAssignments()
}

func (u unitAssigner) Handle(_ <-chan struct{}) error {
	return u.api.AssignUnits()
}
func (unitAssigner) TearDown() error {
	return nil
}
