// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"github.com/juju/juju/api/instancemutater"
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
	}
	return &MutaterMachine{
		context:    w.newMachineContext(),
		logger:     logger,
		machineApi: machine,
		id:         id,
	}
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
