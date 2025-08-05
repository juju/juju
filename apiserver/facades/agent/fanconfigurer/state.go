// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package fanconfigurer

import (
	"github.com/juju/errors"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

// Machine provides access to a machine's base.
type Machine interface {
	Base() state.Base
}

// MachineAccessor provides access to a machine for an id.
type MachineAccessor interface {
	Machine(id string) (Machine, error)
}

// ModelAccessor defines the methods needed to watch for model
// config changes, and read the model config.
type ModelAccessor interface {
	WatchForModelConfigChanges() state.NotifyWatcher
	ModelConfig() (*config.Config, error)
}

type stateShim struct {
	*state.State
}

func (st stateShim) Machine(id string) (Machine, error) {
	m, err := st.State.Machine(id)
	return m, errors.Trace(err)
}
