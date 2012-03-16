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

// agendEmbed is a helper type to embed into those state entities which
// have to provide a defined set of agent related methods.
type agentEmbed struct {
	st     *State
	path   string
	pinger *presence.Pinger
}

// AgentConnected returns true if this entity state has an agent connected.
func (ae *agentEmbed) AgentConnected() (bool, error) {
	return presence.Alive(ae.st.zk, ae.path)
}

// WaitAgentConnected waits until an agent has connected.
func (ae *agentEmbed) WaitAgentConnected() error {
	alive, watch, err := presence.AliveW(ae.st.zk, ae.path)
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

// ConnectAgent informs juju that this associated agent is alive.
func (ae *agentEmbed) ConnectAgent() (err error) {
	if ae.pinger != nil {
		return fmt.Errorf("agent is already connected")
	}
	ae.pinger, err = presence.StartPinger(ae.st.zk, ae.path, agentPingerPeriod)
	return
}

// DisconnectAgent informs juju that this associated agent stops working.
func (ae *agentEmbed) DisconnectAgent() error {
	if ae.pinger == nil {
		return fmt.Errorf("agent is not connected")
	}
	ae.pinger.Kill()
	ae.pinger = nil
	return nil
}
