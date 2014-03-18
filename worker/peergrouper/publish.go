package peergrouper

import (
	"launchpad.net/juju-core/instance"
)

type noPublisher struct{}

func (noPublisher) publishAPIServers(apiServers [][]instance.HostPort) error {
	return nil
}
