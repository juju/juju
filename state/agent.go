// launchpad.net/juju/state
//
// Copyright (c) 2011-2012 Canonical Ltd.
package state

import (
	"fmt"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/tomb"
	"strings"
)

// AgentChange bundles informations about the agent flag changes.
type AgentChange struct {
	Key     string
	Created bool
	Err     error
}

// AgentWatcher delivers notifications when the agent flag
// of an entity state is set or removed.
type AgentWatcher struct {
	Change chan *AgentChange
	tomb   *tomb.Tomb
	st     *State
	path   string
}

// Stop terminates an agent watcher.
func (aw *AgentWatcher) Stop() error {
	aw.tomb.Fatal(tomb.Stop)
	return aw.tomb.Wait()
}

// Err returns a possible error during the watcher execution.
func (aw *AgentWatcher) Err() error {
	return aw.tomb.Err()
}

// loop is receiving the watch events of ZooKeeper and
// re-emits them as agent changes to users of the watch.
func (aw *AgentWatcher) loop() {
	defer aw.tomb.Done()
	defer close(aw.Change)
	// Typical path is /<entity>/<key>/agent. Retrieve
	// the key from it.
	parts := strings.Split(aw.path, "/")
	key := aw.path
	if len(parts) == 4 {
		key = parts[2]
	}
	for {
		_, watch, err := aw.st.zk.ExistsW(aw.path)
		if err != nil && err != zookeeper.ZNONODE {
			aw.Change <- &AgentChange{key, false, err}
		}
		select {
		case <-aw.tomb.Dying:
			return
		case e := <-watch:
			if !e.Ok() {
				aw.Change <- &AgentChange{key, false, fmt.Errorf(e.String())}
			}
			switch e.Type {
			case zookeeper.EVENT_CREATED:
				aw.Change <- &AgentChange{key, true, nil}
			case zookeeper.EVENT_DELETED:
				aw.Change <- &AgentChange{key, false, nil}
			default:
				err := fmt.Errorf("unexpected agent event type: %v", e.Type)
				aw.Change <- &AgentChange{key, false, err}
			}
		}
	}
}

// Agent has to be implemented by state entities that
// will have agents processes. 
type Agent interface {
	// HasAgent returns true if this entity state has an agent connected.
	HasAgent() (bool, error)
	// WatchAgent returns a watcher to observe changes to an agent's presence.
	WatchAgent() *AgentWatcher
	// ConnectAgent informs juju that this associated agent is alive.
	ConnectAgent() error
}

// hasAgent is a helper to implement the HasAgent() method. It
// needs the entities state and the ZooKeeper path for the agent.
func hasAgent(st *State, path string) (bool, error) {
	stat, err := st.zk.Exists(path)
	if err != nil {
		return false, err
	}
	return stat != nil, nil
}

// watchAgent is a helper to implement the WatchAgent() method. It
// needs the entities state and the ZooKeeper path for the agent.
func watchAgent(st *State, path string) *AgentWatcher {
	aw := &AgentWatcher{make(chan *AgentChange, 1), tomb.New(), st, path}
	go aw.loop()
	return aw
}

// connectAgent is a helper to implement the ConnectAgent() method.
// It needs the entities state and the ZooKeeper path for the agent.
func connectAgent(st *State, path string) error {
	_, err := st.zk.Create(path, "", zookeeper.EPHEMERAL, zkPermAll)
	if err != nil && err != zookeeper.ZNODEEXISTS {
		return err
	}
	return nil
}
