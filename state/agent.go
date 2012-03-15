// launchpad.net/juju/state
//
// Copyright (c) 2011-2012 Canonical Ltd.
package state

import (
	"fmt"
	"launchpad.net/juju/go/state/presence"
	"time"
)

const (
	agentPingerPeriod = 25 * time.Millisecond
	agentWaitTimeout  = 4 * agentPingerPeriod
)

// AgentMixin has to be implemented by those state entities
// which will have agent processes.
type AgentMixin interface {
	// AgentConnected returns true if this entity state has an agent connected.
	AgentConnected() (bool, error)
	// WaitAgentConnected waits until an agent has connected.
	WaitAgentConnected() error
	// ConnectAgent informs juju that this associated agent is alive.
	ConnectAgent() error
	// DisconnectAgent informs juju that this associated agent stops working.
	DisconnectAgent() error
}

type agentMixin struct {
	st     *State
	path   string
	pinger *presence.Pinger
}

func newAgentMixin(st *State, path string) *agentMixin {
	return &agentMixin{st, path, nil}
}

// connected is a helper to implement the AgentConnected() method.
func (am *agentMixin) connected() (bool, error) {
	return presence.Alive(am.st.zk, am.path)
}

// waitConnected is a helper to implement the WaitAgentConnected() method.
func (am *agentMixin) waitConnected() error {
	alive, watch, err := presence.AliveW(am.st.zk, am.path)
	if err != nil {
		return err
	}
	// Quick return if already connected.
	if alive {
		return nil
	}
	// Wait for connection with timeout.
	select {
	case alive, ok := <-watch:
		if !ok {
			return fmt.Errorf("wait connection closed")
		}
		if !alive {
			return fmt.Errorf("not connected, must not happen")
		}
	case <-time.After(agentWaitTimeout):
		return fmt.Errorf("wait for connected agent timed out")
	}
	return nil
}

// connectAgent is a helper to implement the ConnectAgent() method.
func (am *agentMixin) connect() (err error) {
	if am.pinger != nil {
		return fmt.Errorf("agent is already connected")
	}
	am.pinger, err = presence.StartPinger(am.st.zk, am.path, agentPingerPeriod)
	return
}

// disconnectAgent is a helper to implement the DisconnectAgent() method.
func (am *agentMixin) disconnect() error {
	if am.pinger == nil {
		return fmt.Errorf("agent is not connected")
	}
	am.pinger.Kill()
	am.pinger = nil
	return nil
}
