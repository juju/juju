// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	"context"
	"reflect"
	"sort"
	"sync"

	"github.com/juju/errors"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
)

// CachingAPIHostPortsSetter is an APIHostPortsSetter that caches the
// most recently set values, suppressing further calls to the underlying
// setter if any call's arguments match those of the preceding call.
type CachingAPIHostPortsSetter struct {
	APIHostPortsSetter

	mu   sync.Mutex
	last []network.SpaceHostPorts
}

func (s *CachingAPIHostPortsSetter) SetAPIHostPorts(controllerConfig controller.Config, apiServers, agentAddresses []network.SpaceHostPorts) error {
	if len(apiServers) == 0 {
		return errors.Errorf("no API servers specified")
	}

	sorted := make([]network.SpaceHostPorts, len(apiServers))
	for i, hostPorts := range apiServers {
		sorted[i] = append(network.SpaceHostPorts{}, hostPorts...)
		sort.Sort(sorted[i])
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if reflect.DeepEqual(sorted, s.last) {
		logger.Debugf(context.TODO(), "API host ports have not changed")
		return nil
	}

	if err := s.APIHostPortsSetter.SetAPIHostPorts(controllerConfig, sorted, sorted); err != nil {
		return errors.Annotate(err, "setting API host ports")
	}
	s.last = sorted
	return nil
}
