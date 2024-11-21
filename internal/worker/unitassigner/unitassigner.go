// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitassigner

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/worker/v4"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

type UnitAssigner interface {
	AssignUnits(ctx context.Context, tags []names.UnitTag) ([]error, error)
	WatchUnitAssignments(ctx context.Context) (watcher.StringsWatcher, error)
	SetAgentStatus(ctx context.Context, args params.SetStatus) error
}

func New(ua UnitAssigner, logger logger.Logger) (worker.Worker, error) {
	return watcher.NewStringsWorker(watcher.StringsConfig{
		Handler: unitAssignerHandler{api: ua, logger: logger},
	})
}

type unitAssignerHandler struct {
	api    UnitAssigner
	logger logger.Logger
}

func (u unitAssignerHandler) SetUp(ctx context.Context) (watcher.StringsWatcher, error) {
	return u.api.WatchUnitAssignments(ctx)
}

func (u unitAssignerHandler) Handle(ctx context.Context, ids []string) error {
	traceEnabled := u.logger.IsLevelEnabled(logger.TRACE)
	if traceEnabled {
		u.logger.Tracef(context.TODO(), "Handling unit assignments: %q", ids)
	}
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

	results, err := u.api.AssignUnits(ctx, units)
	if err != nil {
		return err
	}

	failures := map[string]error{}

	if traceEnabled {
		u.logger.Tracef(context.TODO(), "Unit assignment results: %q", results)
	}
	// errors are returned in the same order as the ids given. Any errors from
	// the assign units call must be reported as error statuses on the
	// respective units (though the assignments will be retried).  Not found
	// errors indicate that the unit was removed before the assignment was
	// requested, which can be safely ignored.
	for i, err := range results {
		if err != nil && !errors.Is(err, errors.NotFound) {
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

		return u.api.SetAgentStatus(ctx, args)
	}
	return nil
}

func (unitAssignerHandler) TearDown() error {
	return nil
}
