// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"errors"
	"time"

	"launchpad.net/tomb"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterStandardFacade("Pinger", 0, NewPinger)
}

// NewPinger returns an object that can be pinged by calling its Ping method.
// If this method is not called frequently enough, the connection will be
// dropped.
func NewPinger(
	st *state.State, resources *common.Resources, authorizer common.Authorizer,
) (
	pinger, error,
) {
	pingTimeout, ok := resources.Get("pingTimeout").(*pingTimeout)
	if !ok {
		return nullPinger{}, nil
	}
	return pingTimeout, nil
}

// pinger describes a type that can be pinged.
type pinger interface {
	Ping()
}

// pingTimeout listens for pings and will call the
// passed action in case of a timeout. This way broken
// or inactive connections can be closed.
type pingTimeout struct {
	tomb    tomb.Tomb
	action  func()
	timeout time.Duration
	reset   chan struct{}
}

// newPingTimeout returns a new pingTimeout instance
// that invokes the given action asynchronously if there
// is more than the given timeout interval between calls
// to its Ping method.
func newPingTimeout(action func(), timeout time.Duration) *pingTimeout {
	pt := &pingTimeout{
		action:  action,
		timeout: timeout,
		reset:   make(chan struct{}),
	}
	go func() {
		defer pt.tomb.Done()
		pt.tomb.Kill(pt.loop())
	}()
	return pt
}

// Ping is used by the client heartbeat monitor and resets
// the killer.
func (pt *pingTimeout) Ping() {
	select {
	case <-pt.tomb.Dying():
	case pt.reset <- struct{}{}:
	}
}

// Stop terminates the ping timeout.
func (pt *pingTimeout) Stop() error {
	pt.tomb.Kill(nil)
	return pt.tomb.Wait()
}

// loop waits for a reset signal, otherwise it performs
// the initially passed action.
func (pt *pingTimeout) loop() error {
	timer := time.NewTimer(pt.timeout)
	defer timer.Stop()
	for {
		select {
		case <-pt.tomb.Dying():
			return nil
		case <-timer.C:
			go pt.action()
			return errors.New("ping timeout")
		case <-pt.reset:
			timer.Reset(pt.timeout)
		}
	}
}

// nullPinger implements the pinger interface but just does nothing
type nullPinger struct{}

func (nullPinger) Ping() {}
