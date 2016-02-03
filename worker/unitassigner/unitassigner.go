// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitassigner

import (
	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker"
	"github.com/juju/loggo"
	"github.com/juju/names"
)

var logger = loggo.GetLogger("juju.worker.unitassigner")

type UnitAssigner interface {
	AssignUnits(tags []names.UnitTag) ([]error, error)
	WatchUnitAssignments() (watcher.StringsWatcher, error)
	SetAgentStatus(args params.SetStatus) error
}

func New(ua UnitAssigner) (worker.Worker, error) {
	return watcher.NewStringsWorker(watcher.StringsConfig{
		Handler: unitAssignerHandler{api: ua},
	})
}

type unitAssignerHandler struct {
	api UnitAssigner
}

func (u unitAssignerHandler) SetUp() (watcher.StringsWatcher, error) {
	return u.api.WatchUnitAssignments()
}

func (u unitAssignerHandler) Handle(_ <-chan struct{}, ids []string) error {
	logger.Tracef("Handling unit assignments: %q", ids)
	if len(ids) == 0 {
		return nil
	}

	units := make([]names.UnitTag, len(ids))
	for i, id := range ids {
		if !names.IsValidUnit(id) {
			return errors.Errorf("%q is not a valid unit id", id)
		}
		units[i] = names.NewUnitTag(id)
	}

	results, err := u.api.AssignUnits(units)
	if err != nil {
		return err
	}

	failures := map[string]error{}

	logger.Tracef("Unit assignment results: %q", results)
	// errors are returned in the same order as the ids given. Any errors from
	// the assign units call must be reported as error statuses on the
	// respective units (though the assignments will be retried).  Not found
	// errors indicate that the unit was removed before the assignment was
	// requested, which can be safely ignored.
	for i, err := range results {
		if err != nil && !errors.IsNotFound(err) {
			failures[units[i].String()] = err
		}
	}

	if len(failures) > 0 {
		args := params.SetStatus{
			Entities: make([]params.EntityStatusArgs, len(failures)),
		}

		x := 0
		for unit, err := range failures {
			args.Entities[x] = params.EntityStatusArgs{
				Tag:    unit,
				Status: params.StatusError,
				Info:   err.Error(),
			}
			x++
		}

		return u.api.SetAgentStatus(args)
	}
	return nil
}

func (unitAssignerHandler) TearDown() error {
	return nil
}
