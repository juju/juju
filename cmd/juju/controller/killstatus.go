// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"fmt"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
	"gopkg.in/juju/names.v2"
)

type ctrData struct {
	UUID               string
	Life               params.Life
	HostedModelCount   int
	HostedMachineCount int
	ServiceCount       int
}

type modelData struct {
	UUID  string
	Owner string
	Name  string
	Life  params.Life

	HostedMachineCount int
	ServiceCount       int
}

// newTimedStatusUpdater returns a function which waits a given period of time
// before querying the apiserver for updated data.
func newTimedStatusUpdater(ctx *cmd.Context, api destroyControllerAPI, controllerModelUUID string) func(time.Duration) (ctrData, []modelData) {
	return func(wait time.Duration) (ctrData, []modelData) {
		time.Sleep(wait)

		// If we hit an error, status.HostedModelCount will be 0, the polling
		// loop will stop and we'll go directly to destroying the model.
		ctrStatus, modelsStatus, err := newData(api, controllerModelUUID)
		if err != nil {
			ctx.Infof("Unable to get the controller summary from the API: %s.", err)
		}

		return ctrStatus, modelsStatus
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

	status, err := api.ModelStatus(names.NewModelTag(controllerModelUUID))
	if err != nil {
		return ctrData{}, nil, errors.Trace(err)
	}
	if l := len(status); l != 1 {
		return ctrData{}, nil, errors.Errorf("error finding controller status: expected one result, got %d", l)
	}
	ctrStatus := status[0]

	hostedModelCount := len(models) - 1
	hostedTags := make([]names.ModelTag, hostedModelCount)
	modelName := map[string]string{}
	var i int
	for _, model := range models {
		if model.UUID != controllerModelUUID {
			modelName[model.UUID] = model.Name
			hostedTags[i] = names.NewModelTag(model.UUID)
			i++
		}
	}

	hostedStatus, err := api.ModelStatus(hostedTags...)
	if err != nil {
		return ctrData{}, nil, errors.Trace(err)
	}

	hostedMachinesCount := ctrStatus.HostedMachineCount
	servicesCount := ctrStatus.ServiceCount
	var modelsData []modelData
	var aliveModelCount int
	for _, model := range hostedStatus {
		if model.Life == params.Dead {
			continue
		}
		modelsData = append(modelsData, modelData{
			model.UUID,
			model.Owner,
			modelName[model.UUID],
			model.Life,
			model.HostedMachineCount,
			model.ServiceCount,
		})

		aliveModelCount++
		hostedMachinesCount += model.HostedMachineCount
		servicesCount += model.ServiceCount
	}

	ctrFinalStatus := ctrData{
		controllerModelUUID,
		ctrStatus.Life,
		aliveModelCount,
		hostedMachinesCount,
		servicesCount,
	}

	return ctrFinalStatus, modelsData, nil
}

func hasUnDeadModels(models []modelData) bool {
	for _, model := range models {
		if model.Life != params.Dead {
			return true
		}
	}
	return false
}

func hasAliveModels(models []modelData) bool {
	for _, model := range models {
		if model.Life == params.Alive {
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

	return out
}
