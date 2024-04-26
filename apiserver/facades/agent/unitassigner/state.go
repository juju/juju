// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitassigner

import (
	"github.com/juju/errors"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state"
)

type stateShim struct {
	*state.State
	prechecker environs.InstancePrechecker
}

func (s stateShim) AssignStagedUnits(allSpaces network.SpaceInfos, ids []string) ([]state.UnitAssignmentResult, error) {
	return s.State.AssignStagedUnits(s.prechecker, allSpaces, ids)
}

func (s stateShim) AssignedMachineId(unit string) (string, error) {
	u, err := s.Unit(unit)
	if err != nil {
		return "", errors.Trace(err)
	}
	return u.AssignedMachineId()
}
