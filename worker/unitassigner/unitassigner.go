// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitassigner

import (
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/worker"
	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.worker.unitassigner")

type UnitAssigner interface {
	AssignUnits(ids []string) ([]error, error)
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
	logger.Tracef("Handling unit assignmewnts: %q", changes)
	if len(changes) == 0 {
		return nil
	}

	// ignore the actual results for now, they'll have been logged on the server
	// side.
	errs, err := u.api.AssignUnits(changes)
	if err != nil {
		return err
	}
	logger.Tracef("Unit assignment results: %q", errs)
	for _, err := range errs {
		if err != nil {
			return err
		}
	}

	return err
}

func (unitAssigner) TearDown() error {
	return nil
}
