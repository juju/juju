package peergrouper

import (
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
)

type publisher struct {
	st *state.State
}

func (pub *publisher) publishAPIServers(apiServers [][]instance.HostPort) error {
	// TODO publish API addresses (instance ids?) in environment storage.
	return pub.st.SetAPIHostPorts(apiServers)
}
