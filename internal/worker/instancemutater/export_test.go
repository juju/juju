// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"

	"github.com/juju/juju/api/agent/instancemutater"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/environs"
)

func NewMachineContext(
	logger logger.Logger,
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

func NewEnvironTestWorker(c *tc.C) func(config Config, ctxFn RequiredMutaterContextFunc) (worker.Worker, error) {
	return func(config Config, ctxFn RequiredMutaterContextFunc) (worker.Worker, error) {
		config.GetMachineWatcher = config.Facade.WatchModelMachines
		config.GetRequiredLXDProfiles = func(modelName string) []string {
			return []string{"default", "juju-" + modelName}
		}
		config.GetRequiredContext = ctxFn
		return newWorker(c.Context(), config)
	}
}

func NewContainerTestWorker(c *tc.C) func(config Config, ctxFn RequiredMutaterContextFunc) (worker.Worker, error) {
	return func(config Config, ctxFn RequiredMutaterContextFunc) (worker.Worker, error) {
		m, err := config.Facade.Machine(c.Context(), config.Tag.(names.MachineTag))
		if err != nil {
			return nil, errors.Trace(err)
		}
		config.GetRequiredLXDProfiles = func(_ string) []string { return []string{"default"} }
		config.GetMachineWatcher = m.WatchContainers
		config.GetRequiredContext = ctxFn
		return newWorker(c.Context(), config)
	}
}

func ProcessMachineProfileChanges(c *tc.C, m *MutaterMachine, info *instancemutater.UnitProfileInfo) error {
	return m.processMachineProfileChanges(c.Context(), info)
}

func GatherProfileData(m *MutaterMachine, info *instancemutater.UnitProfileInfo) ([]lxdprofile.ProfilePost, error) {
	return m.gatherProfileData(info)
}

func VerifyCurrentProfiles(m *MutaterMachine, instId string, expectedProfiles []string) (bool, []string, error) {
	return m.verifyCurrentProfiles(instId, expectedProfiles)
}
