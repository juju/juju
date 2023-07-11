// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
)

// ApplicationDisplayStatus returns the status to display for an application.
func ApplicationDisplayStatus(model *state.Model, app *state.Application, units []*state.Unit) (status.StatusInfo, error) {
	if len(units) == 0 {
		var err error
		units, err = app.AllUnits()
		if err != nil {
			return status.StatusInfo{}, errors.Trace(err)
		}
	}
	statusCtx, err := appStatusContext(model, app, units)
	if err != nil {
		return status.StatusInfo{}, errors.Trace(err)
	}
	return status.DisplayApplicationStatus(*statusCtx), nil
}

func appStatusContext(model *state.Model, application *state.Application, units []*state.Unit) (*status.AppContext, error) {
	var (
		statusCtx status.AppContext
		err       error
	)
	statusCtx.AppStatus, err = application.Status()
	if err != nil {
		return nil, errors.Trace(err)
	}
	statusCtx.OperatorStatus, err = application.OperatorStatus()
	if err != nil && !errors.Is(err, errors.NotFound) {
		return nil, errors.Trace(err)
	}
	statusCtx.IsCaas = model.Type() == state.ModelTypeCAAS
	for _, u := range units {
		workloadStatus, err := u.Status()
		if err != nil {
			return nil, errors.Trace(err)
		}
		containerStatus, err := u.ContainerStatus()
		if err != nil && !errors.Is(err, errors.NotFound) {
			return nil, errors.Trace(err)
		}
		statusCtx.UnitCtx = append(statusCtx.UnitCtx, status.UnitContext{
			WorkloadStatus:  workloadStatus,
			ContainerStatus: containerStatus,
		})
	}
	return &statusCtx, nil
}
