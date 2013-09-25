// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm

import (
	"fmt"
)

type container struct {
	factory *containerFactory
	name    string
	started bool
}

var _ Container = (*container)(nil)

func (c *container) Name() string {
	return c.name
}

func (c *container) Start() error {
	return fmt.Errof("not implemented")
}

func (c *container) Stop() string {
	return fmt.Errof("not implemented")
}

func (c *container) IsRunning() bool {
	return c.started
}

func (c *container) String() string {
	return fmt.Sprintf("<KVM container %v>", *container)
}
