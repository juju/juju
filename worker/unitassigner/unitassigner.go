// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitassigner

import (
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker"
)

type UnitAssigner interface {
	AssignUnits(ids []string) (params.AssignUnitsResults, error)
	WatchUnitAssignments() (watcher.StringsWatcher, error)
}

func New(ua UnitAssigner) worker.Worker {
	return worker.NewStringsWorker(unitAssigner{api: ua})
}

type unitAssigner struct {
	api UnitAssigner
}

func (u unitAssigner) SetUp() (watcher.StringsWatcher, error) {
	return u.api.WatchUnitAssignments()
}

func (u unitAssigner) Handle(changes []string) error {
	if len(changes) == 0 {
		return nil
	}
	// ignore the actual results for now, they'll have been logged on the server
	// side.
	_, err := u.api.AssignUnits(changes)
	return err
}
func (unitAssigner) TearDown() error {
	return nil
}
