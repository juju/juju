// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"github.com/juju/juju/api/instancemutater"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/environs"
	worker "gopkg.in/juju/worker.v1"
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
	config.GetMachineWatcher = config.Facade.WatchMachines
	config.GetRequiredLXDProfiles = func(modelName string) []string {
		return []string{"default", "juju-" + modelName}
	}
	config.GetRequiredContext = ctxFn
	return newWorker(config)
}

func ProcessMachineProfileChanges(m *MutaterMachine, info *instancemutater.UnitProfileInfo) error {
	return m.processMachineProfileChanges(info)
}

func GatherProfileData(m *MutaterMachine, info *instancemutater.UnitProfileInfo) ([]lxdprofile.ProfilePost, error) {
	return m.gatherProfileData(info)
}

func VerifyCurrentProfiles(m *MutaterMachine, instId string, expectedProfiles []string) (bool, error) {
	return m.verifyCurrentProfiles(instId, expectedProfiles)
}
