// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/network"
	corestatus "github.com/juju/juju/core/status"
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
	hasVote   bool
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
		hasVote:   node.HasVote(),
	}
	err := catacomb.Invoke(catacomb.Plan{
		Name: "peergrouper",
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

// HasVote returns whether the controller has a vote (according to
// state).
func (c *controllerTracker) HasVote() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hasVote
}

// Addresses returns the controller addresses from state.
func (c *controllerTracker) Addresses() network.SpaceAddresses {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(network.SpaceAddresses, len(c.addresses))
	copy(out, c.addresses)
	return out
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
		"&peergrouper.controller{id: %q, wantsVote: %v, hasVote: %v, addresses: %v}",
		c.id, c.wantsVote, c.hasVote, c.addresses,
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
		if errors.Is(err, errors.NotFound) {
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
		if errors.Is(err, errors.NotFound) {
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
	// hasVote doesn't count towards a node change,
	// but we still want to record the latest value.
	c.hasVote = c.node.HasVote()

	changed := false
	if wantsVote := c.node.WantsVote(); wantsVote != c.wantsVote {
		c.wantsVote = wantsVote
		changed = true
	}
	return changed, nil
}

func (c *controllerTracker) hostPendingProvisioning() (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	status, err := c.host.Status()
	if err != nil {
		return false, errors.Trace(err)
	}

	return status.Status == corestatus.Pending, nil
}
