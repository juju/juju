// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator

import (
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/state"
)

// StateAccessor exposes State methods needed by credential validator.
type StateAccessor interface {
	Model() (*state.Model, error)
	CloudCredential(tag names.CloudCredentialTag) (state.Credential, error)
	WatchCredential(names.CloudCredentialTag) state.NotifyWatcher
	InvalidateModelCredential(reason string) error
}

type stateShim struct {
	*state.State
}

// NewStateShim creates new state shim to be used by credential validator Facade.
func NewStateShim(st *state.State) StateAccessor {
	return &stateShim{st}
}
