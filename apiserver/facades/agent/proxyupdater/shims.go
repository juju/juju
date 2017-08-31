// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater

import (
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

// stateShim forwards and adapts state.State methods to Backend
// TODO - CAAS(ericclaudejones): This should contain state alone, model will be
// removed once all relevant methods are moved from state to model.
type stateShim struct {
	st *state.State
	m  *state.Model
}

func (s *stateShim) ModelConfig() (*config.Config, error) {
	return s.m.ModelConfig()
}

func (s *stateShim) APIHostPorts() ([][]network.HostPort, error) {
	return s.st.APIHostPorts()
}

func (s *stateShim) WatchAPIHostPorts() state.NotifyWatcher {
	return s.st.WatchAPIHostPorts()
}

func (s *stateShim) WatchForModelConfigChanges() state.NotifyWatcher {
	return s.m.WatchForModelConfigChanges()
}
