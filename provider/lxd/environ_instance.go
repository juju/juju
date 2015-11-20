// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"github.com/juju/errors"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/lxd/lxdclient"
)

// instStatus is the list of statuses to accept when filtering
// for "alive" instances.
var instStatuses = lxdclient.AliveStatuses

// Instances returns the available instances in the environment that
// match the provided instance IDs. For IDs that did not match any
// instances, the result at the corresponding index will be nil. In that
// case the error will be environs.ErrPartialInstances (or
// ErrNoInstances if none of the IDs match an instance).
func (env *environ) Instances(ids []instance.Id) ([]instance.Instance, error) {
	if len(ids) == 0 {
		return nil, environs.ErrNoInstances
	}

	instances, err := getInstances(env)
	if err != nil {
		// We don't return the error since we need to pack one instance
		// for each ID into the result. If there is a problem then we
		// will return either ErrPartialInstances or ErrNoInstances.
		// TODO(ericsnow) Skip returning here only for certain errors?
		logger.Errorf("failed to get instances from LXD: %v", err)
		err = errors.Trace(err)
	}

	// Build the result, matching the provided instance IDs.
	numFound := 0 // This will never be greater than len(ids).
	results := make([]instance.Instance, len(ids))
	for i, id := range ids {
		inst := findInst(id, instances)
		if inst != nil {
			numFound++
		}
		results[i] = inst
	}

	if numFound == 0 {
		if err == nil {
			err = environs.ErrNoInstances
		}
	} else if numFound != len(ids) {
		err = environs.ErrPartialInstances
	}
	return results, err
}

var getInstances = func(env *environ) ([]instance.Instance, error) {
	return env.instances()
}

// instances returns a list of all "alive" instances in the environment.
// This means only instances where the IDs match
// "juju-<env name>-machine-*". This is important because otherwise juju
// will see they are not tracked in state, assume they're stale/rogue,
// and shut them down.
func (env *environ) instances() ([]instance.Instance, error) {
	env = env.getSnapshot()

	prefix := common.MachineFullName(env, "")
	instances, err := env.raw.Instances(prefix, instStatuses...)
	err = errors.Trace(err)

	// Turn lxdclient.Instance values into *environInstance values,
	// whether or not we got an error.
	var results []instance.Instance
	for _, base := range instances {
		// If we don't make a copy then the same pointer is used for the
		// base of all resulting instances.
		copied := base
		inst := newInstance(&copied, env)
		results = append(results, inst)
	}

	return results, err
}

// StateServerInstances returns the IDs of the instances corresponding
// to juju state servers.
func (env *environ) StateServerInstances() ([]instance.Id, error) {
	env = env.getSnapshot()

	prefix := common.MachineFullName(env, "")
	instances, err := env.raw.Instances(prefix, instStatuses...)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var results []instance.Id
	for _, inst := range instances {
		metadata := inst.Metadata()
		isState, ok := metadata[metadataKeyIsState]
		if ok && isState == metadataValueTrue {
			results = append(results, instance.Id(inst.Name))
		}
	}
	if len(results) == 0 {
		return nil, environs.ErrNotBootstrapped
	}
	return results, nil
}

type instPlacement struct{}

func (env *environ) parsePlacement(placement string) (*instPlacement, error) {
	if placement == "" {
		return &instPlacement{}, nil
	}

	return nil, errors.Errorf("unknown placement directive: %v", placement)
}
