// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/gce/google"
)

var instStatuses = []string{
	google.StatusPending,
	google.StatusStaging,
	google.StatusRunning,
}

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
		logger.Errorf("failed to get instances from GCE: %v", err)
		err = errors.Trace(err)
	}

	// Build the result, matching the provided instance IDs.
	numFound := 0 // This will never be greater than len(ids).
	results := make([]instance.Instance, len(ids))
	for i, id := range ids {
		inst := findInst(id, instances)
		if inst != nil {
			numFound += 1
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

func (env *environ) instances() ([]instance.Instance, error) {
	// instances() only returns instances that are part of the
	// environment (instance IDs matches "juju-<env name>-machine-*").
	// This is important because otherwise juju will see they are not
	// tracked in state, assume they're stale/rogue, and shut them down.

	env = env.getSnapshot()

	prefix := common.MachineFullName(env, "")
	instances, err := env.gce.Instances(prefix, instStatuses...)
	err = errors.Trace(err)

	// Turn client.Instance values into *environInstance values,
	// whether or not we got an error.
	var results []instance.Instance
	for _, base := range instances {
		inst := newInstance(&base, env)
		results = append(results, inst)
	}

	return results, err
}

// StateServerInstances returns the IDs of the instances corresponding
// to juju state servers.
func (env *environ) StateServerInstances() ([]instance.Id, error) {
	env = env.getSnapshot()

	prefix := common.MachineFullName(env, "")
	instances, err := env.gce.Instances(prefix, instStatuses...)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var results []instance.Id
	for _, inst := range instances {
		metadata := inst.Metadata()
		isState, ok := metadata[metadataKeyIsState]
		if ok && isState == metadataValueTrue {
			results = append(results, instance.Id(inst.ID))
		}
	}
	if len(results) == 0 {
		return nil, environs.ErrNotBootstrapped
	}
	return results, nil
}

func (env *environ) parsePlacement(placement string) (*google.AvailabilityZone, error) {
	pos := strings.IndexRune(placement, '=')
	if pos == -1 {
		return nil, errors.Errorf("unknown placement directive: %v", placement)
	}
	switch key, value := placement[:pos], placement[pos+1:]; key {
	case "zone":
		zoneName := value
		zones, err := env.AvailabilityZones()
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, z := range zones {
			if z.Name() == zoneName {
				return z.(*google.AvailabilityZone), nil
			}
		}
		return nil, errors.Errorf("invalid availability zone %q", zoneName)
	}
	return nil, errors.Errorf("unknown placement directive: %v", placement)
}

func checkInstanceType(cons constraints.Value) bool {
	// Constraint has an instance-type constraint so let's see if it is valid.
	for _, itype := range allInstanceTypes {
		if itype.Name == *cons.InstanceType {
			return true
		}
	}
	return false
}
