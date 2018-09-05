// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries

import (
	"github.com/juju/juju/core/model"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"
)

type patcher interface {
	PatchValue(interface{}, interface{})
}

func PatchHostSeries(patcher patcher, series string) {
	patcher.PatchValue(&hostSeries, func() (string, error) { return series, nil })
}

func MachineStatus(w worker.Worker) model.UpgradeSeriesStatus {
	return w.(*upgradeSeriesWorker).machineStatus
}

func PreparedUnits(w worker.Worker) []names.UnitTag {
	return w.(*upgradeSeriesWorker).preparedUnits
}

func CompletedUnits(w worker.Worker) []names.UnitTag {
	return w.(*upgradeSeriesWorker).completedUnits
}
