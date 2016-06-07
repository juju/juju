// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container

import (
	"github.com/juju/juju/instance"
	"github.com/juju/names"
)

/// fakeContainerAgentConfig

type fakeContainerAgentConfig struct {
	tag   func() names.Tag
	value func(string) string
}

func (f fakeContainerAgentConfig) Tag() names.Tag {
	if f.tag != nil {
		return f.tag()
	}
	return nil
}

func (f fakeContainerAgentConfig) Value(value string) string {
	if f.value != nil {
		return f.value(value)
	}
	return ""
}

/// fakeContainerManager

func newContainerManagerFn(manager ContainerManager) NewContainerManagerFn {
	return func(instance.ContainerType, ManagerConfig) (ContainerManager, error) {
		return manager, nil
	}
}

type fakeContainerManager struct {
	listContainers func() ([]instance.Instance, error)
	isInitialized  func() bool
}

func (f fakeContainerManager) ListContainers() ([]instance.Instance, error) {
	if f.listContainers != nil {
		return f.listContainers()
	}
	return nil, nil
}

func (f fakeContainerManager) IsInitialized() bool {
	if f.isInitialized != nil {
		return f.isInitialized()
	}
	return true
}
