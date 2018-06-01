// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"github.com/juju/errors"
	"github.com/juju/version"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/tools/lxdclient"
)

// Instances returns the available instances in the environment that
// match the provided instance IDs. For IDs that did not match any
// instances, the result at the corresponding index will be nil. In that
// case the error will be environs.ErrPartialInstances (or
// ErrNoInstances if none of the IDs match an instance).
func (env *environ) Instances(ctx context.ProviderCallContext, ids []instance.Id) ([]instance.Instance, error) {
	if len(ids) == 0 {
		return nil, environs.ErrNoInstances
	}

	instances, err := env.allInstances()
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

func findInst(id instance.Id, instances []*environInstance) instance.Instance {
	for _, inst := range instances {
		if id == inst.Id() {
			return inst
		}
	}
	return nil
}

// instances returns a list of all "alive" instances in the environment.
// We match machine names to the pattern "juju-<model-UUID>-machine-*"
// to ensure that only machines for the environment are returned. This
// is necessary to isolate multiple models within the same LXD.
func (env *environ) allInstances() ([]*environInstance, error) {
	prefix := env.namespace.Prefix()
	insts, err := env.prefixedInstances(prefix)
	return insts, errors.Trace(err)
}

// prefixedInstances returns instances with the specified prefix.
func (env *environ) prefixedInstances(prefix string) ([]*environInstance, error) {
	instances, err := env.raw.Instances(prefix, lxdclient.AliveStatuses...)
	err = errors.Trace(err)

	// Turn lxdclient.Instance values into *environInstance values,
	// whether or not we got an error.
	var results []*environInstance
	for _, base := range instances {
		// If we don't make a copy then the same pointer is used for the
		// base of all resulting instances.
		copied := base
		inst := newInstance(&copied, env)
		results = append(results, inst)
	}
	return results, err
}

// ControllerInstances returns the IDs of the instances corresponding
// to juju controllers.
func (env *environ) ControllerInstances(ctx context.ProviderCallContext, controllerUUID string) ([]instance.Id, error) {
	instances, err := env.raw.Instances("juju-", lxdclient.AliveStatuses...)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var results []instance.Id
	for _, inst := range instances {
		if inst.Metadata()[tags.JujuController] != controllerUUID {
			continue
		}
		if inst.Metadata()[tags.JujuIsController] == "true" {
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

// AdoptResources updates the controller tags on all instances to have the
// new controller id. It's part of the Environ interface.
func (env *environ) AdoptResources(ctx context.ProviderCallContext, controllerUUID string, fromVersion version.Number) error {
	instances, err := env.AllInstances(ctx)
	if err != nil {
		return errors.Annotate(err, "all instances")
	}

	var failed []instance.Id
	qualifiedKey := lxdclient.ResolveConfigKey(tags.JujuController, lxdclient.MetadataNamespace)
	for _, instance := range instances {
		id := instance.Id()
		err := env.raw.UpdateContainerConfig(string(id), map[string]string{qualifiedKey: controllerUUID})
		if err != nil {
			logger.Errorf("error setting controller uuid tag for %q: %v", id, err)
			failed = append(failed, id)
		}
	}
	if len(failed) != 0 {
		return errors.Errorf("failed to update controller for some instances: %v", failed)
	}
	return nil
}
