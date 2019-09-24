// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater

import (
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/config"
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

func (s *stateShim) APIHostPortsForAgents() ([]network.SpaceHostPorts, error) {
	return s.st.APIHostPortsForAgents()
}

func (s *stateShim) WatchAPIHostPortsForAgents() state.NotifyWatcher {
	return s.st.WatchAPIHostPortsForAgents()
}

func (s *stateShim) WatchForModelConfigChanges() state.NotifyWatcher {
	return s.m.WatchForModelConfigChanges()
}
