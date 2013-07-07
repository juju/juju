// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"fmt"
	"time"

	"launchpad.net/loggo"
	"launchpad.net/tomb"

	//"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/common"
	//"launchpad.net/juju-core/state/api/params"
)

var logger = loggo.GetLogger("juju.upgrader")

type upgrader struct {
	tomb        tomb.Tomb
	stateCaller common.Caller
	agentTag    string
}

func NewUpgrader(caller common.Caller, agentTag string) *upgrader {
	u := &upgrader{
		stateCaller: caller,
		agentTag:    agentTag,
	}
	go func() {
		defer u.tomb.Done()
		u.tomb.Kill(u.loop())
	}()
	return u
}

func (u *upgrader) String() string {
	return fmt.Sprintf("upgrader for %q", u.agentTag)
}

// Kill the loop with no-error
func (u *upgrader) Kill() {
	u.tomb.Kill(nil)
}

// Kill and Wait for this to exit
func (u *upgrader) Stop() error {
	u.tomb.Kill(nil)
	return u.tomb.Wait()
}

// Wait for the looping to finish
func (u *upgrader) Wait() error {
	return u.tomb.Wait()
}

func (u *upgrader) loop() error {
	for {
		select {
		case <-u.tomb.Dying():
			return tomb.ErrDying
		case <-time.After(1 * time.Millisecond):
		}
	}
	panic("unreachable")
}
