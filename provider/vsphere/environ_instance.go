// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !gccgo

package vsphere

import (
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/common"
)

// Instances returns the available instances in the environment that
// match the provided instance IDs. For IDs that did not match any
// instances, the result at the corresponding index will be nil. In that
// case the error will be environs.ErrPartialInstances (or
// ErrNoInstances if none of the IDs match an instance).
func (env *environ) Instances(ids []instance.Id) ([]instance.Instance, error) {
	if len(ids) == 0 {
		return nil, environs.ErrNoInstances
	}

	instances, err := env.instances()
	if err != nil {
		// We don't return the error since we need to pack one instance
		// for each ID into the result. If there is a problem then we
		// will return either ErrPartialInstances or ErrNoInstances.
		// TODO(ericsnow) Skip returning here only for certain errors?
		logger.Errorf("failed to get instances from vmware: %v", err)
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

// instances returns a list of all "alive" instances in the environment.
// This means only instances where the IDs match
// "juju-<env name>-machine-*". This is important because otherwise juju
// will see they are not tracked in state, assume they're stale/rogue,
// and shut them down.
func (env *environ) instances() ([]instance.Instance, error) {
	env = env.getSnapshot()

	prefix := common.MachineFullName(env, "")
	instances, err := env.client.Instances(prefix)
	err = errors.Trace(err)

	// Turn mo.VirtualMachine values into *environInstance values,
	// whether or not we got an error.
	var results []instance.Instance
	for _, base := range instances {
		inst := newInstance(base, env)
		results = append(results, inst)
	}

	return results, err
}

// StateServerInstances returns the IDs of the instances corresponding
// to juju state servers.
func (env *environ) StateServerInstances() ([]instance.Id, error) {
	env = env.getSnapshot()

	prefix := common.MachineFullName(env, "")
	instances, err := env.client.Instances(prefix)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var results []instance.Id
	for _, inst := range instances {
		metadata := inst.Config.ExtraConfig
		for _, item := range metadata {
			value := item.GetOptionValue()
			if value.Key == metadataKeyIsState && value.Value == metadataValueIsState {
				results = append(results, instance.Id(inst.Name))
				break
			}
		}
	}
	if len(results) == 0 {
		return nil, environs.ErrNotBootstrapped
	}
	return results, nil
}

// parsePlacement extracts the availability zone from the placement
// string and returns it. If no zone is found there then an error is
// returned.
func (env *environ) parsePlacement(placement string) (*vmwareAvailZone, error) {
	if placement == "" {
		return nil, nil
	}

	pos := strings.IndexRune(placement, '=')
	if pos == -1 {
		return nil, errors.Errorf("unknown placement directive: %v", placement)
	}

	switch key, value := placement[:pos], placement[pos+1:]; key {
	case "zone":
		zone, err := env.availZone(value)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return zone, nil
	}
	return nil, errors.Errorf("unknown placement directive: %v", placement)
}
