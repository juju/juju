// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"context"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/tags"
)

// Instances is part of the environs.Environ interface.
func (env *environ) Instances(ctx context.Context, ids []instance.Id) (instances []instances.Instance, err error) {
	if len(ids) == 0 {
		return nil, environs.ErrNoInstances
	}
	err = env.withSession(ctx, func(env *sessionEnviron) error {
		instances, err = env.Instances(ctx, ids)
		return err
	})
	return instances, err
}

// Instances is part of the environs.Environ interface.
func (env *sessionEnviron) Instances(ctx context.Context, ids []instance.Id) ([]instances.Instance, error) {
	if len(ids) == 0 {
		return nil, environs.ErrNoInstances
	}

	allInstances, err := env.AllRunningInstances(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "failed to get instances")
	}
	findInst := func(id instance.Id) instances.Instance {
		for _, inst := range allInstances {
			if id == inst.Id() {
				return inst
			}
		}
		return nil
	}

	var numFound int
	results := make([]instances.Instance, len(ids))
	for i, id := range ids {
		if inst := findInst(id); inst != nil {
			results[i] = inst
			numFound++
		}
	}
	if numFound == 0 {
		return nil, environs.ErrNoInstances
	} else if numFound != len(ids) {
		err = environs.ErrPartialInstances
	}
	return results, err
}

// ControllerInstances is part of the environs.Environ interface.
func (env *environ) ControllerInstances(ctx envcontext.ProviderCallContext, controllerUUID string) (ids []instance.Id, err error) {
	err = env.withSession(ctx, func(env *sessionEnviron) error {
		ids, err = env.ControllerInstances(ctx, controllerUUID)
		return err
	})
	return ids, err
}

// ControllerInstances is part of the environs.Environ interface.
func (env *sessionEnviron) ControllerInstances(ctx envcontext.ProviderCallContext, controllerUUID string) ([]instance.Id, error) {
	instances, err := env.AllRunningInstances(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var results []instance.Id
	for _, inst := range instances {
		vm := inst.(*environInstance).base
		metadata := vm.Config.ExtraConfig
		var isController bool
		for _, item := range metadata {
			value := item.GetOptionValue()
			if value.Key == tags.JujuIsController && value.Value == "true" {
				isController = true
				break
			}
		}
		if isController {
			results = append(results, inst.Id())
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
func (env *sessionEnviron) parsePlacement(ctx envcontext.ProviderCallContext, placement string) (*vmwareAvailZone, error) {
	if placement == "" {
		return nil, nil
	}

	pos := strings.IndexRune(placement, '=')
	if pos == -1 {
		return nil, errors.Errorf("unknown placement directive: %v", placement)
	}

	switch key, value := placement[:pos], placement[pos+1:]; key {
	case "zone":
		zone, err := env.availZone(ctx, value)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return zone, nil
	}
	return nil, errors.Errorf("unknown placement directive: %v", placement)
}

func (env *sessionEnviron) modelFolderName() string {
	cfg := env.Config()
	return modelFolderName(cfg.UUID(), cfg.Name())
}
