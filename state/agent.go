// launchpad.net/juju/state
//
// Copyright (c) 2011-2012 Canonical Ltd.
package state

import (
	"fmt"
	"launchpad.net/juju/go/state/presence"
	"time"
)

// AgentWatcher monitores the changes of the agent flag of an 
// entity's state.
type AgentWatcher struct {
	watch <-chan bool
}

// IsSet checks if the flag of this agent watcher is set.
// It only waits for a given period to avoid deadlocks.
func (aw *AgentWatcher) IsSet(period time.Duration) (set bool, err error) {
	select {
	case chg, ok := <-aw.watch:
		if !ok {
			return false, fmt.Errorf("watch has been closed")
		}
		return chg, nil
	case <-time.After(period):
		return false, fmt.Errorf("watch timed out")
	}
	return
}

// AgentProcessable has to be implemented by those state entities
// which will have agent processes.
type AgentProcessable interface {
	// HasAgent returns true if this entity state has an agent connected.
	HasAgent() (bool, error)
	// WatchAgent returns a watcher to observe changes to an agent's presence.
	WatchAgent() (*AgentWatcher, error)
	// ConnectAgent informs juju that this associated agent is alive.
	ConnectAgent() error
}

// hasAgent is a helper to implement the HasAgent() method. It
// needs the entity's state and the ZooKeeper path for the agent.
func hasAgent(st *State, path string) (bool, error) {
	return presence.Alive(st.zk, path)
}

// watchAgent is a helper to implement the WatchAgent() method. It
// needs the entity's state and the ZooKeeper path for the agent.
func watchAgent(st *State, path string) (*AgentWatcher, error) {
	_, watch, err := presence.AliveW(st.zk, path)
	if err != nil {
		return nil, err
	}
	return &AgentWatcher{watch}, nil
}

// connectAgent is a helper to implement the ConnectAgent() method.
// It needs the entity's state and the ZooKeeper path for the agent.
func connectAgent(st *State, path string, period time.Duration) error {
	_, err := presence.StartPinger(st.zk, path, period)
	return err
}
