// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils"

	"github.com/juju/juju/rpc"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/apiserver/common"
	"github.com/juju/juju/state/presence"
)

func newStateServer(srv *Server, rpcConn *rpc.Conn, reqNotifier *requestNotifier, limiter utils.Limiter) *initialRoot {
	r := &initialRoot{
		srv:     srv,
		rpcConn: rpcConn,
	}
	r.admin = &srvAdmin{
		root:        r,
		limiter:     limiter,
		validator:   srv.validator,
		reqNotifier: reqNotifier,
	}
	return r
}

// initialRoot implements the API that a client first sees
// when connecting to the API. We start serving a different
// API once the user has logged in.
type initialRoot struct {
	srv     *Server
	rpcConn *rpc.Conn

	admin *srvAdmin
}

// Admin returns an object that provides API access
// to methods that can be called even when not
// authenticated.
func (r *initialRoot) Admin(id string) (*srvAdmin, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return nil, common.ErrBadId
	}
	return r.admin, nil
}

// srvAdmin is the only object that unlogged-in
// clients can access. It holds any methods
// that are needed to log in.
type srvAdmin struct {
	mu          sync.Mutex
	limiter     utils.Limiter
	validator   LoginValidator
	root        *initialRoot
	loggedIn    bool
	reqNotifier *requestNotifier
}

var UpgradeInProgressError = errors.New("upgrade in progress")
var errAlreadyLoggedIn = errors.New("already logged in")

// Login logs in with the provided credentials.
// All subsequent requests on the connection will
// act as the authenticated user.
func (a *srvAdmin) Login(c params.Creds) (params.LoginResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.loggedIn {
		// This can only happen if Login is called concurrently.
		return params.LoginResult{}, errAlreadyLoggedIn
	}

	// Use the login validation function, if one was specified.
	inUpgrade := false
	if a.validator != nil {
		if err := a.validator(c); err != nil {
			if err == UpgradeInProgressError {
				inUpgrade = true
			} else {
				return params.LoginResult{}, errors.Trace(err)
			}
		}
	}

	// Users are not rate limited, all other entities are
	if kind, err := names.TagKind(c.AuthTag); err != nil || kind != names.UserTagKind {
		if !a.limiter.Acquire() {
			logger.Debugf("rate limiting, try again later")
			return params.LoginResult{}, common.ErrTryAgain
		}
		defer a.limiter.Release()
	}
	entity, err := doCheckCreds(a.root.srv.state, c)
	if err != nil {
		return params.LoginResult{}, err
	}
	if a.reqNotifier != nil {
		a.reqNotifier.login(entity.Tag().String())
	}
	// We have authenticated the user; now choose an appropriate API
	// to serve to them.
	var newRoot apiRoot
	if inUpgrade {
		newRoot = newUpgradingRoot(a.root, entity)
	} else {
		newRoot = newSrvRoot(a.root, entity)
	}
	if err := a.startPingerIfAgent(newRoot, entity); err != nil {
		return params.LoginResult{}, err
	}

	// Fetch the API server addresses from state.
	hostPorts, err := a.root.srv.state.APIHostPorts()
	if err != nil {
		return params.LoginResult{}, err
	}
	logger.Debugf("hostPorts: %v", hostPorts)

	environ, err := a.root.srv.state.Environment()
	if err != nil {
		return params.LoginResult{}, err
	}

	a.root.rpcConn.ServeFinder(newRoot, serverError)
	lastConnection := getAndUpdateLastConnectionForEntity(entity)
	return params.LoginResult{
		Servers:        hostPorts,
		EnvironTag:     environ.Tag().String(),
		LastConnection: lastConnection,
		Facades:        newRoot.DescribeFacades(),
	}, nil
}

var doCheckCreds = checkCreds

func checkCreds(st *state.State, c params.Creds) (taggedAuthenticator, error) {
	entity0, err := st.FindEntity(c.AuthTag)
	if err != nil && !errors.IsNotFound(err) {
		return nil, err
	}
	// We return the same error when an entity
	// does not exist as for a bad password, so that
	// we don't allow unauthenticated users to find information
	// about existing entities.
	entity, ok := entity0.(taggedAuthenticator)
	if !ok {
		return nil, common.ErrBadCreds
	}
	if err != nil || !entity.PasswordValid(c.Password) {
		return nil, common.ErrBadCreds
	}
	// Check if a machine agent is logging in with the right Nonce
	if err := checkForValidMachineAgent(entity, c); err != nil {
		return nil, err
	}
	return entity, nil
}

func getAndUpdateLastConnectionForEntity(entity taggedAuthenticator) *time.Time {
	if user, ok := entity.(*state.User); ok {
		result := user.LastConnection()
		user.UpdateLastConnection()
		return result
	}
	return nil
}

func checkForValidMachineAgent(entity taggedAuthenticator, c params.Creds) error {
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

func (a *srvAdmin) startPingerIfAgent(newRoot apiRoot, entity taggedAuthenticator) error {
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

	newRoot.getResources().Register(&machinePinger{pinger})
	action := func() {
		if err := newRoot.getRpcConn().Close(); err != nil {
			logger.Errorf("error closing the RPC connection: %v", err)
		}
	}
	pingTimeout := newPingTimeout(action, maxClientPingInterval)
	newRoot.getResources().RegisterNamed("pingTimeout", pingTimeout)

	return nil
}

// errRoot implements the API that a client first sees
// when connecting to the API. It exposes the same API as initialRoot, except
// it returns the requested error when the client makes any request.
type errRoot struct {
	err error
}

// Admin conforms to the same API as initialRoot, but we'll always return (nil, err)
func (r *errRoot) Admin(id string) (*srvAdmin, error) {
	return nil, r.err
}
