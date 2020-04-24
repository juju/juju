// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/worker/v2/catacomb"

	"github.com/juju/juju/core/network"
)

// controllerTracker is a worker which reports changes of interest to
// the peergrouper for a single controller in state.
type controllerTracker struct {
	catacomb catacomb.Catacomb
	notifyCh chan struct{}
	node     ControllerNode
	host     ControllerHost

	mu sync.Mutex

	// Outside of the controllerTracker implementation itself, these
	// should always be accessed via the getter methods in order to be
	// protected by the mutex.
	id        string
	wantsVote bool
	addresses network.SpaceAddresses
}

func newControllerTracker(node ControllerNode, host ControllerHost, notifyCh chan struct{}) (*controllerTracker, error) {
	m := &controllerTracker{
		notifyCh:  notifyCh,
		id:        node.Id(),
		node:      node,
		host:      host,
		addresses: host.Addresses(),
		wantsVote: node.WantsVote(),
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &m.catacomb,
		Work: m.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return m, nil
}

// Kill implements Worker.
func (c *controllerTracker) Kill() {
	c.catacomb.Kill(nil)
}

// Wait implements Worker.
func (c *controllerTracker) Wait() error {
	return c.catacomb.Wait()
}

// Id returns the id of the controller being tracked.
func (c *controllerTracker) Id() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.id
}

// WantsVote returns whether the controller wants to vote (according to
// state).
func (c *controllerTracker) WantsVote() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.wantsVote
}

// Addresses returns the controller addresses from state.
func (c *controllerTracker) Addresses() network.SpaceAddresses {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(network.SpaceAddresses, len(c.addresses))
	copy(out, c.addresses)
	return out
}

// SelectMongoAddress returns the best address on the controller node for MongoDB peer
// use, using the input space.
func (c *controllerTracker) SelectMongoAddressFromSpace(port int, space network.SpaceInfo) (string, error) {
	if space.ID == "" {
		return "", fmt.Errorf(
			"empty space supplied as an argument for selecting Mongo address for controller node %q", c.id)
	}

	c.mu.Lock()
	hostPorts := network.SpaceAddressesWithPort(c.addresses, port)
	c.mu.Unlock()

	addrs, ok := hostPorts.InSpaces(space)
	if ok {
		addr := network.DialAddress(addrs[0])
		logger.Debugf("controller node %q selected address %q by space %q from %v", c.id, addr, space.Name, hostPorts)
		return addr, nil
	}

	// If we end up here, then there are no addresses available in the
	// specified space. This should not happen, because the configured
	// space is used as a constraint when first enabling HA.
	return "", errors.NotFoundf("addresses for controller node %q in space %q", c.id, space.Name)
}

// GetPotentialMongoHostPorts simply returns all the available addresses
// with the Mongo port appended.
func (c *controllerTracker) GetPotentialMongoHostPorts(port int) network.SpaceHostPorts {
	c.mu.Lock()
	defer c.mu.Unlock()
	return network.SpaceAddressesWithPort(c.addresses, port)
}

func (c *controllerTracker) String() string {
	return c.Id()
}

func (c *controllerTracker) GoString() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	return fmt.Sprintf(
		"&peergrouper.controller{id: %q, wantsVote: %v, addresses: %v}",
		c.id, c.wantsVote, c.addresses,
	)
}

func (c *controllerTracker) loop() error {
	hostWatcher := c.host.Watch()
	if err := c.catacomb.Add(hostWatcher); err != nil {
		return errors.Trace(err)
	}
	nodeWatcher := c.node.Watch()
	if err := c.catacomb.Add(nodeWatcher); err != nil {
		return errors.Trace(err)
	}

	var notifyCh chan struct{}
	for {
		select {
		case <-c.catacomb.Dying():
			return c.catacomb.ErrDying()
		case _, ok := <-hostWatcher.Changes():
			if !ok {
				return hostWatcher.Err()
			}
			changed, err := c.hasHostChanged()
			if err != nil {
				return errors.Trace(err)
			}
			if changed {
				notifyCh = c.notifyCh
			}
		case _, ok := <-nodeWatcher.Changes():
			if !ok {
				return nodeWatcher.Err()
			}
			changed, err := c.hasNodeChanged()
			if err != nil {
				return errors.Trace(err)
			}
			if changed {
				notifyCh = c.notifyCh
			}
		case notifyCh <- struct{}{}:
			notifyCh = nil
		}
	}
}

func (c *controllerTracker) hasHostChanged() (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.host.Refresh(); err != nil {
		if errors.IsNotFound(err) {
			// We want to be robust when the controller
			// state is out of date with respect to the
			// controller info, so if the controller
			// has been removed, just assume that
			// no change has happened - the controller
			// loop will be stopped very soon anyway.
			return false, nil
		}
		return false, errors.Trace(err)
	}
	changed := false
	if addrs := c.host.Addresses(); !reflect.DeepEqual(addrs, c.addresses) {
		c.addresses = addrs
		changed = true
	}
	return changed, nil
}

func (c *controllerTracker) hasNodeChanged() (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.node.Refresh(); err != nil {
		if errors.IsNotFound(err) {
			// We want to be robust when the node
			// state is out of date with respect to the
			// controller info, so if the node
			// has been removed, just assume that
			// no change has happened - the controller
			// loop will be stopped very soon anyway.
			return false, nil
		}
		return false, errors.Trace(err)
	}
	changed := false
	if wantsVote := c.node.WantsVote(); wantsVote != c.wantsVote {
		c.wantsVote = wantsVote
		changed = true
	}
	return changed, nil
}
