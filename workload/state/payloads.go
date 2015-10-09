// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/persistence"
)

// The persistence methods needed for payloads in state.
type payloadsPersistence interface {
	ListPayloads() ([]workload.Payload, error)
}

// EnvPayloads provides the functionality related to an env's
// payloads, as needed by state.
type EnvPayloads struct {
	Base         persistence.PersistenceBase
	ListMachines func() ([]string, error)
	ListUnits    func(string) ([]names.UnitTag, error)
}

// ListAll builds the list of payload information that is registered in state.
func (ps EnvPayloads) ListAll() ([]workload.Payload, error) {
	logger.Tracef("listing all payloads")

	machines, err := ps.ListMachines()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var payloads []workload.Payload
	for _, machine := range machines {
		units, err := ps.ListUnits(machine)
		if err != nil {
			return nil, errors.Trace(err)
		}

		for _, unit := range units {
			persist := persistence.NewPersistence(ps.Base, unit)
			workloads, err := persist.ListAll()
			if err != nil {
				return nil, errors.Trace(err)
			}
			for _, info := range workloads {
				payload := info.AsPayload()
				payload.Unit = unit.String()
				payload.Machine = machine
				payloads = append(payloads, payload)
			}
		}
	}
	return payloads, nil
}
