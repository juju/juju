// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	"sync"

	"github.com/juju/errors"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

type apiHostPortsSetter interface {
	SetAPIHostPorts([][]network.HostPort) error
}

type publisher struct {
	st apiHostPortsSetter

	mu             sync.Mutex
	lastAPIServers [][]network.HostPort
}

func newPublisher(st apiHostPortsSetter) *publisher {
	return &publisher{
		st: st,
	}
}

func (pub *publisher) publishAPIServers(apiServers [][]network.HostPort, instanceIds []instance.Id) error {
	if len(apiServers) == 0 {
		return errors.Errorf("no api servers specified")
	}
	pub.mu.Lock()
	defer pub.mu.Unlock()
	if apiServersEqual(apiServers, pub.lastAPIServers) {
		logger.Debugf("API host ports have not changed")
		return nil
	}
	// TODO(rog) publish instanceIds in environment storage.
	err := pub.st.SetAPIHostPorts(apiServers)
	if err != nil {
		return err
	}
	pub.lastAPIServers = make([][]network.HostPort, len(apiServers))
	for i, hostPorts := range apiServers {
		pub.lastAPIServers[i] = append([]network.HostPort{}, hostPorts...)
	}
	return nil
}

func apiServersEqual(a, b [][]network.HostPort) bool {
	if len(a) != len(b) {
		return false
	}
	for i, hostPortsA := range a {
		hostPortsB := b[i]
		if len(hostPortsA) != len(hostPortsB) {
			return false
		}
		for j := range hostPortsA {
			if hostPortsA[j] != hostPortsB[j] {
				return false
			}
		}
	}
	return true
}
