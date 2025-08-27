// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"strings"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/juju/errors"
	"github.com/juju/version/v2"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/provider/gce/internal/google"
)

// instStatus is the list of statuses to accept when filtering
// for "alive" instances.
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
func (env *environ) Instances(ctx context.ProviderCallContext, ids []instance.Id) ([]instances.Instance, error) {
	if len(ids) == 0 {
		return nil, environs.ErrNoInstances
	}

	all, err := getInstances(env, ctx)
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
	results := make([]instances.Instance, len(ids))
	for i, id := range ids {
		inst := findInst(id, all)
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

var getInstances = func(env *environ, ctx context.ProviderCallContext, statusFilters ...string) ([]instances.Instance, error) {
	return env.instances(ctx, statusFilters...)
}

func (env *environ) gceInstances(ctx context.ProviderCallContext, statusFilters ...string) ([]*computepb.Instance, error) {
	prefix := env.namespace.Prefix()
	if len(statusFilters) == 0 {
		statusFilters = instStatuses
	}
	instances, err := env.gce.Instances(ctx, prefix, statusFilters...)
	return instances, google.HandleCredentialError(errors.Trace(err), ctx)
}

// instances returns a list of all "alive" instances in the environment.
// This means only instances where the IDs match
// "juju-<env name>-machine-*". This is important because otherwise juju
// will see they are not tracked in state, assume they're stale/rogue,
// and shut them down.
func (env *environ) instances(ctx context.ProviderCallContext, statusFilters ...string) ([]instances.Instance, error) {
	gceInstances, err := env.gceInstances(ctx, statusFilters...)
	err = errors.Trace(err)

	// Turn google.Instance values into *environInstance values,
	// whether or not we got an error.
	var results []instances.Instance
	for _, base := range gceInstances {
		inst := newInstance(base, env)
		results = append(results, inst)
	}

	return results, err
}

// unpackMetadata decomposes the provided data from the format used
// in the GCE API.
func unpackMetadata(data *computepb.Metadata) map[string]string {
	if data == nil {
		return nil
	}

	result := make(map[string]string)
	for _, item := range data.Items {
		if item == nil {
			continue
		}
		value := ""
		if item.Value != nil {
			value = *item.Value
		}
		result[item.GetKey()] = value
	}
	return result
}

// ControllerInstances returns the IDs of the instances corresponding
// to juju controllers.
func (env *environ) ControllerInstances(ctx context.ProviderCallContext, controllerUUID string) ([]instance.Id, error) {
	instances, err := env.gceInstances(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var results []instance.Id
	for _, inst := range instances {
		metadata := unpackMetadata(inst.GetMetadata())
		if uuid, ok := metadata[tags.JujuController]; !ok || uuid != controllerUUID {
			continue
		}
		isController, ok := metadata[tags.JujuIsController]
		if ok && isController == "true" {
			results = append(results, instance.Id(inst.GetName()))
		}
	}
	if len(results) == 0 {
		return nil, environs.ErrNotBootstrapped
	}
	return results, nil
}

// AdoptResources is part of the Environ interface.
func (env *environ) AdoptResources(ctx context.ProviderCallContext, controllerUUID string, fromVersion version.Number) error {
	insts, err := env.AllInstances(ctx)
	if err != nil {
		return errors.Annotate(err, "all instances")
	}

	var stringIds []string
	for _, id := range insts {
		stringIds = append(stringIds, string(id.Id()))
	}
	err = env.gce.UpdateMetadata(ctx, tags.JujuController, controllerUUID, stringIds...)
	if err != nil {
		return google.HandleCredentialError(errors.Trace(err), ctx)
	}
	return nil
}

// parsePlacement extracts the availability zone from the placement
// string and returns it. If no zone is found there then an error is
// returned.
func (env *environ) parsePlacement(ctx context.ProviderCallContext, placement string) (*computepb.Zone, error) {
	if placement == "" {
		return nil, nil
	}

	pos := strings.IndexRune(placement, '=')
	if pos == -1 {
		return nil, errors.Errorf("unknown placement directive: %v", placement)
	}

	switch key, value := placement[:pos], placement[pos+1:]; key {
	case "zone":
		zone, err := env.availZoneUp(ctx, value)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return zone, nil
	}
	return nil, errors.Errorf("unknown placement directive: %v", placement)
}

// checkInstanceType is used to ensure the provided constraints
// specify a recognized instance type.
func (env *environ) checkInstanceType(ctx context.ProviderCallContext, cons constraints.Value) bool {
	if cons.InstanceType == nil || *cons.InstanceType == "" {
		return false
	}

	// NOTE(achilleasa): the instance-matching logic in the instances
	// package does not support matching against a instance name so we just
	// fetch all instance types and check manually.
	instTypesAndCosts, err := env.InstanceTypes(ctx, constraints.Value{})
	if err != nil {
		logger.Errorf("unable to fetch GCE instance types: %v", err)
		return false
	}

	for _, itype := range instTypesAndCosts.InstanceTypes {
		if itype.Name == *cons.InstanceType {
			return true
		}
	}
	return false
}
