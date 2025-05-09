// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/internal/cmd"
)

type ctrData struct {
	UUID                 string
	HostedModelCount     int
	HostedMachineCount   int
	ApplicationCount     int
	TotalVolumeCount     int
	TotalFilesystemCount int

	// Model contains controller model data
	Model modelData
}

type modelData struct {
	UUID      string
	Namespace string
	Name      string
	Life      life.Value

	HostedMachineCount        int
	ApplicationCount          int
	VolumeCount               int
	FilesystemCount           int
	PersistentVolumeCount     int
	PersistentFilesystemCount int
}

type environmentStatus struct {
	Controller ctrData
	// models contains only the hosted models. controller.Model
	// contains data specific to the controller model.
	Models       []modelData
	Applications []base.Application
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
		envStatus, err := newData(ctx.Context, api, controllerModelUUID)
		if err != nil {
			ctx.Infof("Unable to get the controller summary from the API: %s.", err)
		}

		return envStatus
	}
}

func newData(ctx context.Context, api destroyControllerAPI, controllerModelUUID string) (environmentStatus, error) {
	models, err := api.AllModels(ctx)
	if err != nil {
		return environmentStatus{
			Controller:   ctrData{},
			Models:       nil,
			Applications: nil,
		}, errors.Trace(err)
	}
	if len(models) == 0 {
		return environmentStatus{
			Controller:   ctrData{},
			Models:       nil,
			Applications: nil,
		}, errors.New("no models found")
	}

	modelTags := make([]names.ModelTag, len(models))
	modelName := make(map[string]string)
	for i, model := range models {
		modelTags[i] = names.NewModelTag(model.UUID)
		modelName[model.UUID] = model.Name
	}

	status, err := api.ModelStatus(ctx, modelTags...)
	if err != nil {
		return environmentStatus{
			Controller:   ctrData{},
			Models:       nil,
			Applications: nil,
		}, errors.Trace(err)
	}

	var hostedMachinesCount int
	var applicationsCount int
	var volumeCount int
	var filesystemCount int
	var modelsData []modelData
	var aliveModelCount int
	var ctrModelData modelData
	var applications []base.Application
	for _, model := range status {
		if model.Error != nil {
			if errors.Is(model.Error, errors.NotFound) {
				// This most likely occurred because a model was
				// destroyed half-way through the call.
				// Since we filter out models with life.Dead below, it's safe
				// to assume that we want to filter these models here too.
				continue
			}
			return environmentStatus{
				Controller:   ctrData{},
				Models:       nil,
				Applications: nil,
			}, errors.Trace(model.Error)
		}
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
			UUID:                      model.UUID,
			Namespace:                 model.Namespace,
			Name:                      modelName[model.UUID],
			Life:                      model.Life,
			HostedMachineCount:        model.HostedMachineCount,
			ApplicationCount:          model.ApplicationCount,
			VolumeCount:               len(model.Volumes),
			FilesystemCount:           len(model.Filesystems),
			PersistentVolumeCount:     persistentVolumeCount,
			PersistentFilesystemCount: persistentFilesystemCount,
		}
		if model.UUID == controllerModelUUID {
			ctrModelData = modelData
		} else {
			if model.Life == life.Dead {
				// Filter out dead, non-controller models.
				continue
			}
			modelsData = append(modelsData, modelData)
			aliveModelCount++
			applications = append(applications, model.Applications...)
		}
		hostedMachinesCount += model.HostedMachineCount
		applicationsCount += model.ApplicationCount
		volumeCount += modelData.VolumeCount
		filesystemCount += modelData.FilesystemCount
	}

	ctrFinalStatus := ctrData{
		UUID:                 controllerModelUUID,
		HostedModelCount:     aliveModelCount,
		HostedMachineCount:   hostedMachinesCount,
		ApplicationCount:     applicationsCount,
		TotalVolumeCount:     volumeCount,
		TotalFilesystemCount: filesystemCount,
		Model:                ctrModelData,
	}

	return environmentStatus{
		Controller:   ctrFinalStatus,
		Models:       modelsData,
		Applications: applications,
	}, nil
}

func hasUnreclaimedResources(env environmentStatus) bool {
	return hasUnDeadModels(env.Models) ||
		env.Controller.HostedMachineCount > 0
}

func hasUnDeadModels(models []modelData) bool {
	for _, model := range models {
		if model.Life != life.Dead {
			return true
		}
	}
	return false
}

func hasAliveModels(models []modelData) bool {
	for _, model := range models {
		if model.Life == life.Alive {
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
	out := fmt.Sprintf("Waiting for %d model%s", modelNo, s(modelNo))

	if machineNo := data.HostedMachineCount; machineNo > 0 {
		out += fmt.Sprintf(", %d machine%s", machineNo, s(machineNo))
	}

	if applicationNo := data.ApplicationCount; applicationNo > 0 {
		out += fmt.Sprintf(", %d application%s", applicationNo, s(applicationNo))
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
	out := fmt.Sprintf("\t%s/%s (%s)", data.Namespace, data.Name, data.Life)

	if machineNo := data.HostedMachineCount; machineNo > 0 {
		out += fmt.Sprintf(", %d machine%s", machineNo, s(machineNo))
	}

	if applicationNo := data.ApplicationCount; applicationNo > 0 {
		out += fmt.Sprintf(", %d application%s", applicationNo, s(applicationNo))
	}

	if n := data.VolumeCount; n > 0 {
		out += fmt.Sprintf(", %d volume%s", n, s(n))
	}

	if n := data.FilesystemCount; n > 0 {
		out += fmt.Sprintf(", %d filesystem%s", n, s(n))
	}

	return out
}
