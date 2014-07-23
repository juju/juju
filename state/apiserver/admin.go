// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/rpc"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/apiserver/authentication"
	"github.com/juju/juju/state/apiserver/common"
	"github.com/juju/juju/state/presence"
)

// adminApiV1 implements the API that a client first sees when connecting to
// the API. We start serving a different API once the user has logged in.
type adminApiV1 struct {
	admin *adminV1
}

// adminV1 is the only object that unlogged-in clients can access. It holds any
// methods that are needed to log in.
type adminV1 struct {
	srv         *Server
	root        *ApiHandler
	reqNotifier *requestNotifier
	mu          sync.Mutex
	loggedIn    bool
}

func newAdminApiV1(srv *Server, root *ApiHandler, reqNotifier *requestNotifier) *adminApiV1 {
	return &adminApiV1{
		admin: &adminV1{
			srv:         srv,
			root:        root,
			reqNotifier: reqNotifier,
		},
	}
}

// Admin returns an object that provides API access to methods that can be
// called even when not authenticated.
func (r *adminApiV1) Admin(id string) (*adminV1, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return nil, common.ErrBadId
	}
	return r.admin, nil
}

var UpgradeInProgressError = errors.New("upgrade in progress")
var errAlreadyLoggedIn = errors.New("already logged in")

// Login logs in with the provided credentials.  All subsequent requests on the
// connection will act as the authenticated user.
func (a *adminV1) Login(c params.Creds) (params.LoginResult, error) {
	var fail params.LoginResult

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.loggedIn {
		// This can only happen if Login is called concurrently.
		return fail, errAlreadyLoggedIn
	}

	var authApi rpc.MethodFinder = NewApiRoot(a.srv, a.root.resources, a.root)

	// Use the login validation function, if one was specified.
	if a.srv.validator != nil {
		if err := a.srv.validator(c); err != nil {
			if err == UpgradeInProgressError {
				authApi = NewUpgradingRoot(authApi)
			} else {
				return fail, errors.Trace(err)
			}
		}
	}

	// Users are not rate limited, all other entities are
	if kind, err := names.TagKind(c.AuthTag); err != nil || kind != names.UserTagKind {
		if !a.srv.limiter.Acquire() {
			logger.Debugf("rate limiting, try again later")
			return fail, common.ErrTryAgain
		}
		defer a.srv.limiter.Release()
	}

	entity, err := doCheckCreds(a.srv.state, c)
	if err != nil {
		return fail, err
	}
	a.root.entity = entity
	if a.reqNotifier != nil {
		a.reqNotifier.login(entity.Tag().String())
	}

	// We have authenticated the user; enable the appropriate API
	// to serve to them.
	a.loggedIn = true
	a.root.MethodFinder = authApi

	if err := a.startPingerIfAgent(entity); err != nil {
		return fail, err
	}

	// Fetch the API server addresses from state.
	hostPorts, err := a.root.state.APIHostPorts()
	if err != nil {
		return fail, err
	}
	logger.Debugf("hostPorts: %v", hostPorts)

	environ, err := a.root.state.Environment()
	if err != nil {
		return fail, err
	}

	lastConnection := getAndUpdateLastConnectionForEntity(entity)
	return params.LoginResult{
		Servers:        hostPorts,
		EnvironTag:     environ.Tag().String(),
		LastConnection: lastConnection,
		Facades:        a.root.DescribeFacades(),
	}, nil
}

var doCheckCreds = checkCreds

func checkCreds(st *state.State, c params.Creds) (state.Entity, error) {
	entity, err := st.FindEntity(c.AuthTag)
	if errors.IsNotFound(err) {
		// We return the same error when an entity does not exist as for a bad
		// password, so that we don't allow unauthenticated users to find
		// information about existing entities.
		return nil, common.ErrBadCreds
	}
	if err != nil {
		return nil, errors.Trace(err)
	}

	authenticator, err := authentication.FindEntityAuthenticator(entity)
	if err != nil {
		return nil, err
	}

	if err = authenticator.Authenticate(entity, c.Password, c.Nonce); err != nil {
		return nil, err
	}

	return entity, nil
}

func getAndUpdateLastConnectionForEntity(entity state.Entity) *time.Time {
	if user, ok := entity.(*state.User); ok {
		result := user.LastConnection()
		user.UpdateLastConnection()
		return result
	}
	return nil
}

func checkForValidMachineAgent(entity state.Entity, c params.Creds) error {
	// If this is a machine agent connecting, we need to check the
	// nonce matches, otherwise the wrong agent might be trying to
	// connect.
	if machine, ok := entity.(*state.Machine); ok {
		if !machine.CheckProvisioned(c.Nonce) {
			return state.NotProvisionedError(machine.Id())
		}
	}
	return nil
}

// machinePinger wraps a presence.Pinger.
type machinePinger struct {
	*presence.Pinger
}

// Stop implements Pinger.Stop() as Pinger.Kill(), needed at
// connection closing time to properly stop the wrapped pinger.
func (p *machinePinger) Stop() error {
	if err := p.Pinger.Stop(); err != nil {
		return err
	}
	return p.Pinger.Kill()
}

func (a *adminV1) startPingerIfAgent(entity state.Entity) error {
	// A machine or unit agent has connected, so start a pinger to
	// announce it's now alive, and set up the API pinger
	// so that the connection will be terminated if a sufficient
	// interval passes between pings.
	agentPresencer, ok := entity.(presence.Presencer)
	if !ok {
		return nil
	}

	pinger, err := agentPresencer.SetAgentPresence()
	if err != nil {
		return err
	}

	a.root.getResources().Register(&machinePinger{pinger})
	action := func() {
		if err := a.root.getRpcConn().Close(); err != nil {
			logger.Errorf("error closing the RPC connection: %v", err)
		}
	}
	pingTimeout := newPingTimeout(action, maxClientPingInterval)
	a.root.getResources().RegisterNamed("pingTimeout", pingTimeout)

	return nil
}

// errRoot implements the API that a client first sees
// when connecting to the API. It exposes the same API as initialRoot, except
// it returns the requested error when the client makes any request.
type errRoot struct {
	err error
}

// Admin conforms to the same API as initialRoot, but we'll always return (nil, err)
func (r *errRoot) Admin(id string) (*adminV1, error) {
	return nil, r.err
}
