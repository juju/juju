// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitassigner

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
)

type stateShim struct {
	*state.State
	modelConfigService common.ModelConfigService
}

func (s stateShim) AssignStagedUnits(allSpaces network.SpaceInfos, ids []string) ([]state.UnitAssignmentResult, error) {
	return s.State.AssignStagedUnits(s.modelConfigService, allSpaces, ids)
}

func (s stateShim) AssignedMachineId(unit string) (string, error) {
	u, err := s.Unit(unit)
	if err != nil {
		return "", errors.Trace(err)
	}
	return u.AssignedMachineId()
}
