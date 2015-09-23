// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitassigner

import (
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker"
)

type UnitAssigner interface {
	AssignUnits() (params.AssignUnitsResults, error)
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
	// ignore the actual results for now, they'll have been logged on the server
	// side.
	_, err := u.api.AssignUnits()
	return err
}
func (unitAssigner) TearDown() error {
	return nil
}
