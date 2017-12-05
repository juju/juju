// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner

type ContainerBroker interface {
	EnsureUnit(unitName, spec string) error
}
