// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	stderrors "errors"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/rpc"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	"launchpad.net/juju-core/state/presence"
	"sync"
)

func newStateServer(srv *Server, rpcConn *rpc.Conn) *initialRoot {
	r := &initialRoot{
		srv:     srv,
		rpcConn: rpcConn,
	}
	r.admin = &srvAdmin{
		root: r,
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
	mu       sync.Mutex
	root     *initialRoot
	loggedIn bool
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
	entity, err := a.root.srv.state.Authenticator(c.AuthTag)
	if err != nil && !errors.IsNotFoundError(err) {
		return err
	}
	// We return the same error when an entity
	// does not exist as for a bad password, so that
	// we don't allow unauthenticated users to find information
	// about existing entities.
	if err != nil || !entity.PasswordValid(c.Password) {
		return common.ErrBadCreds
	}
	// We have authenticated the user; now choose an appropriate API
	// to serve to them.
	newRoot, err := a.apiRootForEntity(entity, c)
	if err != nil {
		return err
	}

	if err := a.root.rpcConn.Serve(newRoot, serverError); err != nil {
		return err
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
	return p.Pinger.Kill()
}

func (a *srvAdmin) apiRootForEntity(entity state.TaggedAuthenticator, c params.Creds) (interface{}, error) {
	// TODO(rog) choose appropriate object to serve.
	newRoot := newSrvRoot(a.root.srv, entity)

	// If this is a machine agent connecting, we need to check the
	// nonce matches, otherwise the wrong agent might be trying to
	// connect.
	machine, ok := entity.(*state.Machine)
	if ok {
		if !machine.CheckProvisioned(c.Nonce) {
			return nil, common.ErrNotProvisioned
		}
		// The machine agent has connected, so start a pinger to announce
		// it's now alive.
		pinger, err := machine.SetAgentAlive()
		if err != nil {
			return nil, err
		}
		newRoot.resources.Register(&machinePinger{pinger})
	}
	return newRoot, nil
}
