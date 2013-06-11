// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container

// A Container represents a containerized virtual machine.
type Container interface {
	Create() error
	Start() error
	Stop() error
	Destroy() error
}
