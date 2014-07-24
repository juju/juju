// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"reflect"

	"github.com/juju/juju/rpc"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/apiserver/common"
)

var (
	RootType              = reflect.TypeOf(&apiHandler{})
	NewPingTimeout        = newPingTimeout
	MaxClientPingInterval = &maxClientPingInterval
	MongoPingInterval     = &mongoPingInterval
	UploadBackupToStorage = &uploadBackupToStorage
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
	delayedCheckCreds := func(st *state.State, c params.Creds) (state.Entity, error) {
		<-nextChan
		return checkCreds(st, c)
	}
	doCheckCreds = delayedCheckCreds
	return
}

func NewErrRoot(err error) *errRoot {
	return &errRoot{err}
}

// TestingApiHandler gives you an ApiHandler that is *barely* connected to anything.
// Just enough to let you probe some of the interfaces of ApiHandler, but not
// enough to actually do any RPC calls
func TestingApiRoot(st *state.State) rpc.MethodFinder {
	srv := &Server{state: st}
	h := newApiRoot(srv, common.NewResources(), nil)
	return h
}

// TestingUpgradingApiHandler returns a limited srvRoot
// in an upgrade scenario.
func TestingUpgradingRoot(st *state.State) rpc.MethodFinder {
	r := TestingApiRoot(st)
	return newUpgradingRoot(r)
}
