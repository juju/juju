// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"fmt"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils/clock"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
)

type ctrData struct {
	UUID                 string
	HostedModelCount     int
	HostedMachineCount   int
	ServiceCount         int
	TotalVolumeCount     int
	TotalFilesystemCount int

	// Model contains controller model data
	Model modelData
}

type modelData struct {
	UUID  string
	Owner string
	Name  string
	Life  string

	HostedMachineCount        int
	ServiceCount              int
	VolumeCount               int
	FilesystemCount           int
	PersistentVolumeCount     int
	PersistentFilesystemCount int
}

type environmentStatus struct {
	controller ctrData
	// models contains only the hosted models. controller.Model
	// contains data specific to the controller model.
	models []modelData
}

// newTimedStatusUpdater returns a function which waits a given period of time
// before querying the apiserver for updated data.
func newTimedStatusUpdater(ctx *cmd.Context, api destroyControllerAPI, controllerModelUUID string, clock clock.Clock) func(time.Duration) environmentStatus {
	return func(wait time.Duration) environmentStatus {
		if wait > 0 {
			<-clock.After(wait)
		}

		// If we hit an error, status.HostedModelCount will be 0, the polling
		// loop will stop and we'll go directly to destroying the model.
		ctrStatus, modelsStatus, err := newData(api, controllerModelUUID)
		if err != nil {
			ctx.Infof("Unable to get the controller summary from the API: %s.", err)
		}

		return environmentStatus{
			controller: ctrStatus,
			models:     modelsStatus,
		}
	}
}

func newData(api destroyControllerAPI, controllerModelUUID string) (ctrData, []modelData, error) {
	models, err := api.AllModels()
	if err != nil {
		return ctrData{}, nil, errors.Trace(err)
	}
	if len(models) == 0 {
		return ctrData{}, nil, errors.New("no models found")
	}

	modelTags := make([]names.ModelTag, len(models))
	modelName := make(map[string]string)
	for i, model := range models {
		modelTags[i] = names.NewModelTag(model.UUID)
		modelName[model.UUID] = model.Name
	}

	status, err := api.ModelStatus(modelTags...)
	if err != nil {
		return ctrData{}, nil, errors.Trace(err)
	}

	var hostedMachinesCount int
	var servicesCount int
	var volumeCount int
	var filesystemCount int
	var modelsData []modelData
	var aliveModelCount int
	var ctrModelData modelData
	for _, model := range status {
		var persistentVolumeCount int
		var persistentFilesystemCount int
		for _, v := range model.Volumes {
			if v.Detachable {
				persistentVolumeCount++
			}
		}
		for _, f := range model.Filesystems {
			if f.Detachable {
				persistentFilesystemCount++
			}
		}
		modelData := modelData{
			model.UUID,
			model.Owner,
			modelName[model.UUID],
			model.Life,
			model.HostedMachineCount,
			model.ServiceCount,
			len(model.Volumes),
			len(model.Filesystems),
			persistentVolumeCount,
			persistentFilesystemCount,
		}
		if model.UUID == controllerModelUUID {
			ctrModelData = modelData
		} else {
			if model.Life == string(params.Dead) {
				// Filter out dead, non-controller models.
				continue
			}
			modelsData = append(modelsData, modelData)
			aliveModelCount++
		}
		hostedMachinesCount += model.HostedMachineCount
		servicesCount += model.ServiceCount
		volumeCount += modelData.VolumeCount
		filesystemCount += modelData.FilesystemCount
	}

	ctrFinalStatus := ctrData{
		controllerModelUUID,
		aliveModelCount,
		hostedMachinesCount,
		servicesCount,
		volumeCount,
		filesystemCount,
		ctrModelData,
	}

	return ctrFinalStatus, modelsData, nil
}

func hasUnreclaimedResources(env environmentStatus) bool {
	return hasUnDeadModels(env.models) ||
		env.controller.HostedMachineCount > 0
}

func hasUnDeadModels(models []modelData) bool {
	for _, model := range models {
		if model.Life != string(params.Dead) {
			return true
		}
	}
	return false
}

func hasAliveModels(models []modelData) bool {
	for _, model := range models {
		if model.Life == string(params.Alive) {
			return true
		}
	}
	return false
}

func s(n int) string {
	if n > 1 {
		return "s"
	}
	return ""
}

func fmtCtrStatus(data ctrData) string {
	modelNo := data.HostedModelCount
	out := fmt.Sprintf("Waiting on %d model%s", modelNo, s(modelNo))

	if machineNo := data.HostedMachineCount; machineNo > 0 {
		out += fmt.Sprintf(", %d machine%s", machineNo, s(machineNo))
	}

	if serviceNo := data.ServiceCount; serviceNo > 0 {
		out += fmt.Sprintf(", %d application%s", serviceNo, s(serviceNo))
	}

	if n := data.TotalVolumeCount; n > 0 {
		out += fmt.Sprintf(", %d volume%s", n, s(n))
	}

	if n := data.TotalFilesystemCount; n > 0 {
		out += fmt.Sprintf(", %d filesystem%s", n, s(n))
	}

	return out
}

func fmtModelStatus(data modelData) string {
	out := fmt.Sprintf("\t%s/%s (%s)", data.Owner, data.Name, data.Life)

	if machineNo := data.HostedMachineCount; machineNo > 0 {
		out += fmt.Sprintf(", %d machine%s", machineNo, s(machineNo))
	}

	if serviceNo := data.ServiceCount; serviceNo > 0 {
		out += fmt.Sprintf(", %d application%s", serviceNo, s(serviceNo))
	}

	if n := data.VolumeCount; n > 0 {
		out += fmt.Sprintf(", %d volume%s", n, s(n))
	}

	if n := data.FilesystemCount; n > 0 {
		out += fmt.Sprintf(", %d filesystem%s", n, s(n))
	}

	return out
}
