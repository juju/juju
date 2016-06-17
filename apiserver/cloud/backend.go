// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/state"
)

type Backend interface {
	Cloud() (cloud.Cloud, error)
	CloudCredentials(names.UserTag) (map[string]cloud.Credential, error)
	UpdateCloudCredentials(names.UserTag, map[string]cloud.Credential) error

	IsControllerAdministrator(names.UserTag) (bool, error)

	Close() error
}

type stateShim struct {
	*state.State
}

func NewStateBackend(st *state.State) Backend {
	return stateShim{st}
}
