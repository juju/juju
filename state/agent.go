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
	agentPingerPeriod = 1 * time.Second
)

// agent represents a juju agent.
type agent struct {
	st     *State
	path   string
	pinger *presence.Pinger
}

// Connected returns true if its entity state has an agent connected.
func (a *agent) Connected() (bool, error) {
	return presence.Alive(a.st.zk, a.path)
}

// WaitConnected waits until an agent has connected.
func (a *agent) WaitConnected(timeout time.Duration) error {
	alive, watch, err := presence.AliveW(a.st.zk, a.path)
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
	case <-time.After(timeout):
		return fmt.Errorf("wait for connected agent timed out")
	}
	return nil
}

// Connect informs juju that an associated agent is alive.
func (a *agent) Connect() (err error) {
	if a.pinger != nil {
		return fmt.Errorf("agent is already connected")
	}
	a.pinger, err = presence.StartPinger(a.st.zk, a.path, agentPingerPeriod)
	return
}

// Disconnect informs juju that an associated agent stops working.
func (a *agent) Disconnect() error {
	if a.pinger == nil {
		return fmt.Errorf("agent is not connected")
	}
	a.pinger.Kill()
	a.pinger = nil
	return nil
}
