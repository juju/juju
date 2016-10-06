// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import "github.com/juju/utils/clock"

// monitor performs regular pings of an API connection as well as
// monitoring the connection closed channel and the underlying
// rpc.Conn's dead channel. It will close `broken` if pings fail, or
// if `closed` or `dead` are closed.
type monitor struct {
	clock  clock.Clock
	ping   func() error
	closed <-chan struct{}
	dead   <-chan struct{}
	broken chan<- struct{}
}

func (m *monitor) run() {
	defer close(m.broken)
	for {
		select {
		case <-m.closed:
			return
		case <-m.dead:
			return
		case <-m.clock.After(PingPeriod):
			if !m.pingWithTimeout() {
				return
			}
		}
	}
}

func (m *monitor) pingWithTimeout() bool {
	result := make(chan error, 1)
	go func() {
		// Note that result is buffered so that we don't leak this
		// goroutine when a timeout happens.
		result <- m.ping()
	}()
	select {
	case err := <-result:
		if err != nil {
			logger.Debugf("health ping failed: %v", err)
		}
		return err == nil
	case <-m.clock.After(PingTimeout):
		logger.Errorf("health ping timed out after %s", PingTimeout)
		return false
	}
}
