// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/juju/errors"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/worker/catacomb"
)

// machineTracker is a worker which reports changes of interest to
// the peergrouper for a single machine in state.
type machineTracker struct {
	catacomb catacomb.Catacomb
	notifyCh chan struct{}
	stm      Machine

	mu sync.Mutex

	// Outside of the machineTracker implementation itself, these
	// should always be accessed via the getter methods in order to be
	// protected by the mutex.
	id        string
	wantsVote bool
	addresses []network.Address
}

func newMachineTracker(stm Machine, notifyCh chan struct{}) (*machineTracker, error) {
	m := &machineTracker{
		notifyCh:  notifyCh,
		id:        stm.Id(),
		stm:       stm,
		addresses: stm.Addresses(),
		wantsVote: stm.WantsVote(),
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
func (m *machineTracker) Kill() {
	m.catacomb.Kill(nil)
}

// Wait implements Worker.
func (m *machineTracker) Wait() error {
	return m.catacomb.Wait()
}

// Id returns the id of the machine being tracked.
func (m *machineTracker) Id() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.id
}

// WantsVote returns whether the machine wants to vote (according to
// state).
func (m *machineTracker) WantsVote() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.wantsVote
}

// Addresses returns the machine addresses from state.
func (m *machineTracker) Addresses() []network.Address {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]network.Address, len(m.addresses))
	copy(out, m.addresses)
	return out
}

// SelectMongoHostPort returns the best hostport for the machine for
// MongoDB use, perhaps using the space provided.
func (m *machineTracker) SelectMongoHostPort(port int, space network.SpaceName) string {
	m.mu.Lock()
	hostPorts := network.AddressesWithPort(m.addresses, port)
	m.mu.Unlock()

	if space != "" {
		return mongo.SelectPeerHostPortBySpace(hostPorts, space)
	}
	return mongo.SelectPeerHostPort(hostPorts)
}

func (m *machineTracker) String() string {
	return m.Id()
}

func (m *machineTracker) GoString() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	return fmt.Sprintf(
		"&peergrouper.machine{id: %q, wantsVote: %v, addresses: %v}",
		m.id, m.wantsVote, m.addresses,
	)
}

func (m *machineTracker) loop() error {
	watcher := m.stm.Watch()
	if err := m.catacomb.Add(watcher); err != nil {
		return errors.Trace(err)
	}

	var notifyCh chan struct{}
	for {
		select {
		case <-m.catacomb.Dying():
			return m.catacomb.ErrDying()
		case _, ok := <-watcher.Changes():
			if !ok {
				return watcher.Err()
			}
			changed, err := m.hasChanged()
			if err != nil {
				return errors.Trace(err)
			}
			if changed {
				notifyCh = m.notifyCh
			}
		case notifyCh <- struct{}{}:
			notifyCh = nil
		}
	}
}

func (m *machineTracker) hasChanged() (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.stm.Refresh(); err != nil {
		if errors.IsNotFound(err) {
			// We want to be robust when the machine
			// state is out of date with respect to the
			// controller info, so if the machine
			// has been removed, just assume that
			// no change has happened - the machine
			// loop will be stopped very soon anyway.
			return false, nil
		}
		return false, errors.Trace(err)
	}
	changed := false
	if wantsVote := m.stm.WantsVote(); wantsVote != m.wantsVote {
		m.wantsVote = wantsVote
		changed = true
	}
	if addrs := m.stm.Addresses(); !reflect.DeepEqual(addrs, m.addresses) {
		m.addresses = addrs
		changed = true
	}
	return changed, nil
}
