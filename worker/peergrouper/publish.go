// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	"reflect"
	"sync"

	"github.com/juju/errors"

	"github.com/juju/juju/network"
)

// CachingAPIHostPortsSetter is an APIHostPortsSetter that caches the
// most recently set values, suppressing further calls to the underlying
// setter if any call's arguments match those of the preceding call.
type CachingAPIHostPortsSetter struct {
	APIHostPortsSetter

	mu   sync.Mutex
	last [][]network.HostPort
}

func (s *CachingAPIHostPortsSetter) SetAPIHostPorts(apiServers [][]network.HostPort) error {
	if len(apiServers) == 0 {
		return errors.Errorf("no API servers specified")
	}

	sorted := make([][]network.HostPort, len(apiServers))
	for i, hostPorts := range apiServers {
		sorted[i] = append([]network.HostPort{}, hostPorts...)
		network.SortHostPorts(sorted[i])
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if reflect.DeepEqual(sorted, s.last) {
		logger.Debugf("API host ports have not changed")
		return nil
	}

	if err := s.APIHostPortsSetter.SetAPIHostPorts(sorted); err != nil {
		return errors.Annotate(err, "setting API host ports")
	}
	s.last = sorted
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
