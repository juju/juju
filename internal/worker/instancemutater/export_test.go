// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/worker/v4"

	"github.com/juju/juju/api/agent/instancemutater"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/environs"
)

func NewMachineContext(
	logger Logger,
	broker environs.LXDProfiler,
	machine instancemutater.MutaterMachine,
	fn RequiredLXDProfilesFunc,
	id string,
) *MutaterMachine {
	w := mutaterWorker{
		broker:                     broker,
		getRequiredLXDProfilesFunc: fn,
		getRequiredContextFunc: func(w MutaterContext) MutaterContext {
			return w
		},
	}
	return &MutaterMachine{
		context:    w.newMachineContext(),
		logger:     logger,
		machineApi: machine,
		id:         id,
	}
}

func NewEnvironTestWorker(config Config, ctxFn RequiredMutaterContextFunc) (worker.Worker, error) {
	config.GetMachineWatcher = config.Facade.WatchModelMachines
	config.GetRequiredLXDProfiles = func(modelName string) []string {
		return []string{"default", "juju-" + modelName}
	}
	config.GetRequiredContext = ctxFn
	return newWorker(context.Background(), config)
}

func NewContainerTestWorker(config Config, ctxFn RequiredMutaterContextFunc) (worker.Worker, error) {
	m, err := config.Facade.Machine(context.Background(), config.Tag.(names.MachineTag))
	if err != nil {
		return nil, errors.Trace(err)
	}
	config.GetRequiredLXDProfiles = func(_ string) []string { return []string{"default"} }
	config.GetMachineWatcher = m.WatchContainers
	config.GetRequiredContext = ctxFn
	return newWorker(context.Background(), config)
}

func ProcessMachineProfileChanges(m *MutaterMachine, info *instancemutater.UnitProfileInfo) error {
	return m.processMachineProfileChanges(context.Background(), info)
}

func GatherProfileData(m *MutaterMachine, info *instancemutater.UnitProfileInfo) ([]lxdprofile.ProfilePost, error) {
	return m.gatherProfileData(info)
}

func VerifyCurrentProfiles(m *MutaterMachine, instId string, expectedProfiles []string) (bool, error) {
	return m.verifyCurrentProfiles(instId, expectedProfiles)
}
