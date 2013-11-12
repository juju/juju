// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm

import (
	"fmt"
)

type containerFactory struct {
}

var _ ContainerFactory = (*containerFactory)(nil)

func (factory *containerFactory) New(name string) Container {
	return &kvmContainer{
		factory: factory,
		name:    name,
	}
}

func (factory *containerFactory) List() ([]Container, error) {
	return nil, fmt.Errorf("Not yet implemented")
}
