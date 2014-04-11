// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"reflect"

	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

var (
	RootType              = reflect.TypeOf(&srvRoot{})
	NewPingTimeout        = newPingTimeout
	MaxClientPingInterval = &maxClientPingInterval
	MongoPingInterval     = &mongoPingInterval
)

const LoginRateLimit = loginRateLimit

// DelayLogins changes how the Login code works so that logins won't proceed
// until they get a message on the returned channel.
// After calling this function, the caller is responsible for sending messages
// on the nextChan in order for Logins to succeed. The original behavior can be
// restored by calling the cleanup function.
func DelayLogins() (nextChan chan struct{}, cleanup func()) {
	nextChan = make(chan struct{}, 10)
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
