// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater

// stateShim forwards and adapts state.State methods to Backing
import (
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

// method.
type stateShim struct {
	State
	st *state.State
}

func (s *stateShim) EnvironConfig() (*config.Config, error) {
	return s.st.EnvironConfig()
}

func (s *stateShim) APIHostPorts() ([][]network.HostPort, error) {
	return s.st.APIHostPorts()
}

func (s *stateShim) WatchAPIHostPorts() state.NotifyWatcher {
	return s.st.WatchAPIHostPorts()
}

func (s *stateShim) WatchForEnvironConfigChanges() state.NotifyWatcher {
	return s.st.WatchForEnvironConfigChanges()
}
