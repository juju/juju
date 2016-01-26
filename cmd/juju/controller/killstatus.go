// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"fmt"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/names"
)

type ctrData struct {
	HostedEnvCount     int
	HostedMachineCount int
	ServiceCount       int
}

type envData struct {
	Owner string
	Name  string
	Life  params.Life

	HostedMachineCount int
	ServiceCount       int
}

// newTimedStatusUpdater returns a function which waits a given period of time
// before querying the apiserver for updated data.
func newTimedStatusUpdater(ctx *cmd.Context, api destroyControllerAPI, uuid string) func(time.Duration) (ctrData, []envData) {
	return func(wait time.Duration) (ctrData, []envData) {
		time.Sleep(wait)

		// If we hit an error, status.HostedEnvCount will be 0, the polling
		// loop will stop and we'll go directly to destroying the environment.
		ctrStatus, envsStatus, err := newData(api, uuid)
		if err != nil {
			ctx.Infof("Unable to get the controller summary from the API: %s.", err)
		}

		return ctrStatus, envsStatus
	}
}

func newData(api destroyControllerAPI, ctrUUID string) (ctrData, []envData, error) {
	envs, err := api.AllModels()
	if err != nil {
		return ctrData{}, nil, errors.Trace(err)
	}
	if len(envs) == 0 {
		return ctrData{}, nil, errors.New("no models found")
	}

	status, err := api.ModelStatus(names.NewModelTag(ctrUUID))
	if err != nil {
		return ctrData{}, nil, errors.Trace(err)
	}
	if l := len(status); l != 1 {
		return ctrData{}, nil, errors.Errorf("error finding controller status: expected one result, got %d", l)
	}
	ctrStatus := status[0]

	hostedEnvCount := len(envs) - 1
	hostedTags := make([]names.ModelTag, hostedEnvCount)
	envName := map[string]string{}
	var i int
	for _, env := range envs {
		if env.UUID != ctrUUID {
			envName[env.UUID] = env.Name
			hostedTags[i] = names.NewModelTag(env.UUID)
			i++
		}
	}

	hostedStatus, err := api.ModelStatus(hostedTags...)
	if err != nil {
		return ctrData{}, nil, errors.Trace(err)
	}

	hostedMachinesCount := ctrStatus.HostedMachineCount
	servicesCount := ctrStatus.ServiceCount
	var envsData []envData
	var aliveEnvCount int
	for _, env := range hostedStatus {
		if env.Life == params.Dead {
			continue
		}
		envsData = append(envsData, envData{
			env.Owner,
			envName[env.UUID],
			env.Life,
			env.HostedMachineCount,
			env.ServiceCount,
		})

		aliveEnvCount++
		hostedMachinesCount += env.HostedMachineCount
		servicesCount += env.ServiceCount
	}

	ctrFinalStatus := ctrData{
		aliveEnvCount,
		hostedMachinesCount,
		servicesCount,
	}

	return ctrFinalStatus, envsData, nil
}

func hasUnDeadEnvirons(envs []envData) bool {
	for _, env := range envs {
		if env.Life != params.Dead {
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
	envNo := data.HostedEnvCount
	out := fmt.Sprintf("Waiting on %d model%s", envNo, s(envNo))

	if machineNo := data.HostedMachineCount; machineNo > 0 {
		out += fmt.Sprintf(", %d machine%s", machineNo, s(machineNo))
	}

	if serviceNo := data.ServiceCount; serviceNo > 0 {
		out += fmt.Sprintf(", %d service%s", serviceNo, s(serviceNo))
	}

	return out
}

func fmtEnvStatus(data envData) string {
	out := fmt.Sprintf("%s/%s (%s)", data.Owner, data.Name, data.Life)

	if machineNo := data.HostedMachineCount; machineNo > 0 {
		out += fmt.Sprintf(", %d machine%s", machineNo, s(machineNo))
	}

	if serviceNo := data.ServiceCount; serviceNo > 0 {
		out += fmt.Sprintf(", %d service%s", serviceNo, s(serviceNo))
	}

	return out
}
