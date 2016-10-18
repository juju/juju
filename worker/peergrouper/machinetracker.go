// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	"fmt"
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
	stm      stateMachine

	mu sync.Mutex

	// Outside of the machineTracker implementation itself, these
	// should always be accessed via the getter methods in order to be
	// protected by the mutex.
	id             string
	wantsVote      bool
	apiHostPorts   []network.HostPort
	mongoHostPorts []network.HostPort
}

func newMachineTracker(stm stateMachine, notifyCh chan struct{}) (*machineTracker, error) {
	m := &machineTracker{
		notifyCh:       notifyCh,
		id:             stm.Id(),
		stm:            stm,
		apiHostPorts:   stm.APIHostPorts(),
		mongoHostPorts: stm.MongoHostPorts(),
		wantsVote:      stm.WantsVote(),
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

// WantsVote returns the MongoDB hostports from state.
func (m *machineTracker) MongoHostPorts() []network.HostPort {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]network.HostPort, len(m.mongoHostPorts))
	copy(out, m.mongoHostPorts)
	return out
}

// APIHostPorts returns the API server hostports from state.
func (m *machineTracker) APIHostPorts() []network.HostPort {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]network.HostPort, len(m.apiHostPorts))
	copy(out, m.apiHostPorts)
	return out
}

// SelectMongoHostPort returns the best hostport for the machine for
// MongoDB use, perhaps using the space provided.
func (m *machineTracker) SelectMongoHostPort(mongoSpace network.SpaceName) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if mongoSpace != "" {
		return mongo.SelectPeerHostPortBySpace(m.mongoHostPorts, mongoSpace)
	}
	return mongo.SelectPeerHostPort(m.mongoHostPorts)
}

func (m *machineTracker) String() string {
	return m.Id()
}

func (m *machineTracker) GoString() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	return fmt.Sprintf("&peergrouper.machine{id: %q, wantsVote: %v, hostPorts: %v}",
		m.id, m.wantsVote, m.mongoHostPorts)
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
	if hps := m.stm.MongoHostPorts(); !hostPortsEqual(hps, m.mongoHostPorts) {
		m.mongoHostPorts = hps
		changed = true
	}
	if hps := m.stm.APIHostPorts(); !hostPortsEqual(hps, m.apiHostPorts) {
		m.apiHostPorts = hps
		changed = true
	}
	return changed, nil
}

func hostPortsEqual(hps1, hps2 []network.HostPort) bool {
	if len(hps1) != len(hps2) {
		return false
	}
	for i := range hps1 {
		if hps1[i] != hps2[i] {
			return false
		}
	}
	return true
}
