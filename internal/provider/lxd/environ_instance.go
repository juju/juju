// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/version/v2"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/internal/container/lxd"
)

// Instances returns the available instances in the environment that
// match the provided instance IDs. For IDs that did not match any
// instances, the result at the corresponding index will be nil. In that
// case the error will be environs.ErrPartialInstances (or
// ErrNoInstances if none of the IDs match an instance).
func (env *environ) Instances(ctx context.Context, ids []instance.Id) ([]instances.Instance, error) {
	if len(ids) == 0 {
		return nil, environs.ErrNoInstances
	}

	all, err := env.allInstances()
	if err != nil {
		// We don't return the error since we need to pack one instance
		// for each ID into the result. If there is a problem then we
		// will return either ErrPartialInstances or ErrNoInstances.
		// TODO(ericsnow) Skip returning here only for certain errors?
		logger.Errorf(ctx, "failed to get instances from LXD: %v", err)
		err = errors.Trace(env.HandleCredentialError(ctx, err))
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

func findInst(id instance.Id, instances []*environInstance) instances.Instance {
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
	containers, err := env.server().AliveContainers(prefix)
	err = errors.Trace(err)

	// Turn lxd.Container values into *environInstance values,
	// whether or not we got an error.
	var results []*environInstance
	for _, c := range containers {
		c := c
		inst := newInstance(&c, env)
		results = append(results, inst)
	}
	return results, err
}

// ControllerInstances returns the IDs of the instances corresponding
// to juju controllers.
func (env *environ) ControllerInstances(ctx envcontext.ProviderCallContext, controllerUUID string) ([]instance.Id, error) {
	containers, err := env.server().AliveContainers("juju-")
	if err != nil {
		return nil, errors.Trace(env.HandleCredentialError(ctx, err))
	}

	var results []instance.Id
	for _, c := range containers {
		if c.Metadata(tags.JujuController) != controllerUUID {
			continue
		}
		if c.Metadata(tags.JujuIsController) == "true" {
			results = append(results, instance.Id(c.Name))
		}
	}
	if len(results) == 0 {
		return nil, environs.ErrNotBootstrapped
	}
	return results, nil
}

// AdoptResources updates the controller tags on all instances to have the
// new controller id. It's part of the Environ interface.
func (env *environ) AdoptResources(ctx envcontext.ProviderCallContext, controllerUUID string, fromVersion version.Number) error {
	instances, err := env.AllInstances(ctx)
	if err != nil {
		return errors.Annotate(env.HandleCredentialError(ctx, err), "all instances")
	}

	var failed []instance.Id
	qualifiedKey := lxd.UserNamespacePrefix + tags.JujuController
	for _, instance := range instances {
		id := instance.Id()
		// TODO (manadart 2018-06-27) This is a smell.
		// Everywhere else, we update the container config on a container and then call WriteContainer.
		// If we added a method directly to environInstance to do this, we wouldn't need this
		// implementation of UpdateContainerConfig at all, and the container representation we are
		// holding would be consistent with that on the server.
		err := env.server().UpdateContainerConfig(string(id), map[string]string{qualifiedKey: controllerUUID})
		if err != nil {
			logger.Errorf(ctx, "error setting controller uuid tag for %q: %v", id, err)
			failed = append(failed, id)
		}
	}
	if len(failed) != 0 {
		return errors.Errorf("failed to update controller for some instances: %v", failed)
	}
	return nil
}
