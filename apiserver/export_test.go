// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"fmt"
	"reflect"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/state"
	"github.com/juju/names"
)

var (
	RootType              = reflect.TypeOf(&apiHandler{})
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
	delayedCheckCreds := func(st *state.State, c params.LoginRequest) (state.Entity, error) {
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

type preFacadeAdminApi struct{}

func newPreFacadeAdminApi(srv *Server, root *apiHandler, reqNotifier *requestNotifier) interface{} {
	return &preFacadeAdminApi{}
}

func (r *preFacadeAdminApi) Admin(id string) (*preFacadeAdminApi, error) {
	return r, nil
}

var PreFacadeEnvironTag = names.NewEnvironTag("383c49f3-526d-4f9e-b50a-1e6fa4e9b3d9")

func (r *preFacadeAdminApi) Login(c params.Creds) (params.LoginResult, error) {
	return params.LoginResult{
		EnvironTag: PreFacadeEnvironTag.String(),
	}, nil
}

type failAdminApi struct{}

func newFailAdminApi(srv *Server, root *apiHandler, reqNotifier *requestNotifier) interface{} {
	return &failAdminApi{}
}

func (r *failAdminApi) Admin(id string) (*failAdminApi, error) {
	return r, nil
}

func (r *failAdminApi) Login(c params.Creds) (params.LoginResult, error) {
	return params.LoginResult{}, fmt.Errorf("fail")
}

// SetPreFacadeAdminApi is used to create a test scenario where the API server
// does not know about API facade versioning. In this case, the client should
// login to the v1 facade, which sends backwards-compatible login fields.
// The v0 facade will fail on a pre-defined error.
func SetPreFacadeAdminApi(srv *Server) {
	srv.adminApiFactories = map[int]adminApiFactory{
		0: newFailAdminApi,
		1: newPreFacadeAdminApi,
	}
}

func SetAdminApiVersions(srv *Server, versions ...int) {
	factories := make(map[int]adminApiFactory)
	for _, n := range versions {
		switch n {
		case 0:
			factories[n] = newAdminApiV0
		case 1:
			factories[n] = newAdminApiV1
		default:
			panic(fmt.Errorf("unknown admin API version %d", n))
		}
	}
	srv.adminApiFactories = factories
}

// TestingRestoreInProgressRoot returns a limited restoreInProgressRoot
// containing a srvRoot as returned by TestingSrvRoot.
func TestingRestoreInProgressRoot(st *state.State) *restoreInProgressRoot {
	r := TestingApiRoot(st)
	return newRestoreInProgressRoot(r)
}

// TestingAboutToRestoreRoot returns a limited aboutToRestoreRoot
// containing a srvRoot as returned by TestingSrvRoot.
func TestingAboutToRestoreRoot(st *state.State) *aboutToRestoreRoot {
	r := TestingApiRoot(st)
	return newAboutToRestoreRoot(r)
}
