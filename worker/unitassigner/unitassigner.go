// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitassigner

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
)

// logger is here to stop the desire of creating a package level logger.
// Don't do this, instead use the one passed as manifold config.
var logger interface{}

type UnitAssigner interface {
	AssignUnits(tags []names.UnitTag) ([]error, error)
	WatchUnitAssignments() (watcher.StringsWatcher, error)
	SetAgentStatus(args params.SetStatus) error
}

func New(ua UnitAssigner, logger Logger) (worker.Worker, error) {
	return watcher.NewStringsWorker(watcher.StringsConfig{
		Handler: unitAssignerHandler{api: ua, logger: logger},
	})
}

type unitAssignerHandler struct {
	api    UnitAssigner
	logger Logger
}

func (u unitAssignerHandler) SetUp() (watcher.StringsWatcher, error) {
	return u.api.WatchUnitAssignments()
}

func (u unitAssignerHandler) Handle(_ <-chan struct{}, ids []string) error {
	u.logger.Tracef("Handling unit assignments: %q", ids)
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

	u.logger.Tracef("Unit assignment results: %q", results)
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
				Status: status.Error.String(),
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
