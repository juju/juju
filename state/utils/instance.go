// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/state"
)

var getEnvironment = GetEnvironment

// AvailabilityZone returns the availability zone associated with
// an instance ID.
func AvailabilityZone(st *state.State, instID instance.Id) (string, error) {
	// Get the provider.
	env, err := getEnvironment(st)
	if err != nil {
		return "", errors.Trace(err)
	}
	zenv, ok := env.(common.ZonedEnviron)
	if !ok {
		return "", errors.NotSupportedf(`zones for provider "%T"`, env)
	}

	// Request the zone.
	zones, err := zenv.InstanceAvailabilityZoneNames([]instance.Id{instID})
	if err != nil {
		return "", errors.Trace(err)
	}
	if len(zones) != 1 {
		return "", errors.Errorf("received invalid zones: expected 1, got %d", len(zones))
	}

	return zones[0], nil
}

func machineID(st *state.State, tag names.Tag) (string, error) {
	switch tag := tag.(type) {
	case names.UnitTag:
		unit, err := st.Unit(tag.Id())
		if err != nil {
			return "", errors.Trace(err)
		}
		mid, err := unit.AssignedMachineId()
		if err != nil {
			return "", errors.Annotatef(err, "unit %q has no assigned machine", unit)
		}
		return mid, nil
	case names.MachineTag:
		return tag.Id(), nil
	default:
		return "", errors.Errorf("unsupported tag type: %v", tag)
	}
}

// InstanceID returns the instance ID for the given tag.
func InstanceID(st *state.State, tag names.Tag) (instance.Id, error) {
	mid, err := machineID(st, tag)
	if err != nil {
		return "", errors.Trace(err)
	}
	machine, err := st.Machine(mid)
	if err != nil {
		return "", errors.Trace(err)
	}
	instID, err = machine.InstanceId()
	return instID, errors.Trace(err)
}
