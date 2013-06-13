// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container

import (
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
)

// A Container represents a containerized virtual machine.
type Container interface {
	Name() string
	Instance() environs.Instance
	Create(
		series, nonce string,
		tools *state.Tools,
		environConfig *config.Config,
		stateInfo *state.Info,
		apiInfo *api.Info) error
	Start() error
	Stop() error
	Destroy() error
}
