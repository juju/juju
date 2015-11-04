// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"

	"github.com/juju/juju/workload"
)

// EnvPayloads provides the functionality related to an env's
// payloads, as needed by state.
type EnvPayloads struct {
	UnitListFuncs func() ([]func() ([]workload.Info, string, string, error), error)
}

// ListAll builds the list of payload information that is registered in state.
func (ps EnvPayloads) ListAll() ([]workload.Payload, error) {
	logger.Tracef("listing all payloads")

	funcs, err := ps.UnitListFuncs()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var payloads []workload.Payload
	for _, listAll := range funcs {
		workloads, machine, unit, err := listAll()
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, info := range workloads {
			payload := info.AsPayload()
			payload.Unit = unit
			payload.Machine = machine
			payloads = append(payloads, payload)
		}
	}
	return payloads, nil
}
