// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm

import (
	"fmt"

	"launchpad.net/juju-core/container"
)

type kvmContainer struct {
	factory *containerFactory
	name    string
	started bool
}

var _ container.Container = (*kvmContainer)(nil)

func (c *kvmContainer) Name() string {
	return c.name
}

func (c *kvmContainer) Start() error {
	return fmt.Errorf("not implemented")
}

func (c *kvmContainer) Stop() error {
	return fmt.Errorf("not implemented")
}

func (c *kvmContainer) IsRunning() bool {
	return c.started
}

func (c *kvmContainer) String() string {
	return fmt.Sprintf("<KVM container %v>", *c)
}
