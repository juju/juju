// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm

import (
	"fmt"

	"launchpad.net/juju-core/container"
)

type containerFactory struct {
}

var _ container.ContainerFactory = (*containerFactory)(nil)

func (factory *containerFactory) New(name string) container.Container {
	return &kvmContainer{
		factory: factory,
		name:    name,
	}
}

func (factory *containerFactory) List() ([]container.Container, error) {
	return nil, fmt.Errorf("Not yet implemented")
}
