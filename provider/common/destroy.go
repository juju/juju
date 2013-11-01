// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"launchpad.net/juju-core/environs"
)

// Destroy is a common implementation of the Destroy method defined on
// environs.Environ; we strongly recommend that this implementation be
// used when writing a new provider.
func Destroy(env environs.Environ) error {
	logger.Infof("destroying environment %q", env.Name())
	instances, err := env.AllInstances()
	switch err {
	case nil:
		if err := env.StopInstances(instances); err != nil {
			return err
		}
		fallthrough
	case environs.ErrNoInstances:
		return env.Storage().RemoveAll()
	}
	return err
}
