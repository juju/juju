// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation

import (
	"github.com/juju/errors"
	"github.com/juju/replicaset/v3"

	"github.com/juju/juju/state"
)

type upgradevalidationShim struct {
	*state.State
}

func NewUpgradeValidationShim(st *state.State) State {
	return &upgradevalidationShim{st}
}

func (s *upgradevalidationShim) AllCharmURLs() ([]*string, error) {
	return s.State.AllCharmURLs()
}

func (s *upgradevalidationShim) HasUpgradeSeriesLocks() (bool, error) {
	return s.State.HasUpgradeSeriesLocks()
}

func (s *upgradevalidationShim) MachineCountForBase(base ...state.Base) (map[string]int, error) {
	return s.State.MachineCountForBase(base...)
}

func (s *upgradevalidationShim) MongoCurrentStatus() (*replicaset.Status, error) {
	return nil, errors.NotImplementedf("this is not used but just for implementing the interface")
}

func (s *upgradevalidationShim) AllMachines() ([]Machine, error) {
	machines, err := s.State.AllMachines()
	if err != nil {
		return nil, errors.Trace(err)
	}
	out := make([]Machine, len(machines))
	for i, machine := range machines {
		out[i] = machine
	}
	return out, nil
}
