package peergrouper

import (
	"launchpad.net/juju-core/instance"
)

//type publisher struct {
//	st *state.State
//}
//
//func (pub *publisher) publishAPIServers(apiServers []instance.HostPort) error {
//	return pub.st.SetAPIAddresses(
//}

type noPublisher struct{}

func (noPublisher) publishAPIServers(apiServers [][]instance.HostPort) error {
	return nil
}
