// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
)

func (env *environ) StartInstance(args environs.StartInstanceParams) (
	instance.Instance, *instance.HardwareCharacteristics, []environs.NetworkInfo, error,
) {
	// Please note that in order to fulfil the demands made of Instances and
	// AllInstances, it is imperative that some environment feature be used to
	// keep track of which instances were actually started by juju.
	_ = env.getSnapshot()
	return nil, nil, nil, errNotImplemented
}

func (env *environ) AllInstances() ([]instance.Instance, error) {
	// Please note that this must *not* return instances that have not been
	// allocated as part of this environment -- if it does, juju will see they
	// are not tracked in state, assume they're stale/rogue, and shut them down.
	_ = env.getSnapshot()
	return nil, errNotImplemented
}

func (env *environ) Instances(ids []instance.Id) ([]instance.Instance, error) {
	// Please note that this must *not* return instances that have not been
	// allocated as part of this environment -- if it does, juju will see they
	// are not tracked in state, assume they're stale/rogue, and shut them down.
	// This advice applies even if an instance id passed in corresponds to a
	// real instance that's not part of the environment -- the Environ should
	// treat that no differently to a request for one that does not exist.
	_ = env.getSnapshot()
	return nil, errNotImplemented
}

func (env *environ) StopInstances(instances []instance.Instance) error {
	_ = env.getSnapshot()
	return errNotImplemented
}
