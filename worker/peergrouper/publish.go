package peergrouper

import (
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/instance"
)

type publisher struct {
	st *state.State
}

func (pub *publisher) publishAPIServers(apiServers [][]instance.HostPort) error {
	// TODO publish API addresses (instance ids?) in environment.
	return pub.st.SetAPIHostPorts(apiServers)
}
