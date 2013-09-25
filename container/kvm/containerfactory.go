// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm

import (
	"fmt"

	base "launchpad.net/juju-core/container"
)

type containerFactory struct {
}

var _ base.ContainerFactory = (*containerFactory)(nil)

func (factory *containerFactory) New(name string) base.Container {
	return &container{
		factory: factory,
		name:    name,
	}
}

func (factory *containerFactory) List() ([]base.Container, error) {
	return nil, fmt.Errorf("Not yet implemented")
}
