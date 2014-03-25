// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	stderrors "errors"
	"sync"

	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/rpc"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	"launchpad.net/juju-core/state/presence"
)

func newStateServer(srv *Server, rpcConn *rpc.Conn, reqNotifier *requestNotifier) *initialRoot {
	r := &initialRoot{
		srv:     srv,
		rpcConn: rpcConn,
	}
	r.admin = &srvAdmin{
		root:        r,
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
	root        *initialRoot
	loggedIn    bool
	reqNotifier *requestNotifier
}

var errAlreadyLoggedIn = stderrors.New("already logged in")

// Login logs in with the provided credentials.
// All subsequent requests on the connection will
// act as the authenticated user.
func (a *srvAdmin) Login(c params.Creds) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.loggedIn {
		// This can only happen if Login is called concurrently.
		return errAlreadyLoggedIn
	}
	entity, err := checkCreds(a.root.srv.state, c)
	if err != nil {
		return err
	}
	if a.reqNotifier != nil {
		a.reqNotifier.login(entity.Tag())
	}
	// We have authenticated the user; now choose an appropriate API
	// to serve to them.
	newRoot, err := a.apiRootForEntity(entity, c)
	if err != nil {
		return err
	}

	a.root.rpcConn.Serve(newRoot, serverError)
	return nil
}

func checkCreds(st *state.State, c params.Creds) (taggedAuthenticator, error) {
	entity0, err := st.FindEntity(c.AuthTag)
	if err != nil && !errors.IsNotFoundError(err) {
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
	return entity, nil
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

func (a *srvAdmin) apiRootForEntity(entity taggedAuthenticator, c params.Creds) (interface{}, error) {
	// TODO(rog) choose appropriate object to serve.
	newRoot := newSrvRoot(a.root, entity)

	// If this is a machine agent connecting, we need to check the
	// nonce matches, otherwise the wrong agent might be trying to
	// connect.
	machine, ok := entity.(*state.Machine)
	if ok {
		if !machine.CheckProvisioned(c.Nonce) {
			return nil, state.NotProvisionedError(machine.Id())
		}
	}
	setAgentAliver, ok := entity.(interface {
		SetAgentAlive() (*presence.Pinger, error)
	})
	if ok {
		// A machine or unit agent has connected, so start a pinger to
		// announce it's now alive, and set up the API pinger
		// so that the connection will be terminated if a sufficient
		// interval passes between pings.
		pinger, err := setAgentAliver.SetAgentAlive()
		if err != nil {
			return nil, err
		}
		newRoot.resources.Register(&machinePinger{pinger})
		action := func() {
			if err := newRoot.rpcConn.Close(); err != nil {
				logger.Errorf("error closing the RPC connection: %v", err)
			}
		}
		newRoot.pingTimeout = newPingTimeout(action, maxPingInterval)
	}
	return newRoot, nil
}
