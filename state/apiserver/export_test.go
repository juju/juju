// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"reflect"

	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

var (
	RootType        = reflect.TypeOf(&srvRoot{})
	NewPingTimeout  = newPingTimeout
	MaxPingInterval = &maxPingInterval
)

// DelayCheckCreds overwrites the internal structures with an alternative
// checkCreds implementation. The new implementation waits on a channel before
// actually processing credentials. After calling this function, Logins will
// not be processed until a message is sent down nextChan. Restore the original
// behavior by calling the cleanup() function.
func DelayCheckCreds() (nextChan chan struct{}, cleanup func()) {
	nextChan = make(chan struct{}, 0)
	cleanup = func() {
		doCheckCreds = checkCreds
	}
	delayedCheckCreds := func(st *state.State, c params.Creds) (taggedAuthenticator, error) {
		<-nextChan
		return checkCreds(st, c)
	}
	doCheckCreds = delayedCheckCreds
	return
}
