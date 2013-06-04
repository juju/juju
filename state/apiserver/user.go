// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"sync"
)

// srvUser serves API methods on a state User.
type srvUser struct {
	root *srvRoot
	u    *state.User
}

// SetPassword sets the user's password.
func (u *srvUser) SetPassword(p params.Password) error {
	return setPassword(u.u, p.Password)
}

// Get retrieves all details of a user.
func (u *srvUser) Get() (params.User, error) {
	return params.User{}, nil
}

// authUser holds login details. It's ok to call
// its methods concurrently.
type authUser struct {
	mu     sync.Mutex
	entity state.TaggedAuthenticator // logged-in entity (access only when mu is locked)
}

// login authenticates as entity with the given name,.
func (u *authUser) login(st *state.State, tag, password string) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	entity, err := st.Authenticator(tag)
	if err != nil && !errors.IsNotFoundError(err) {
		return err
	}
	// We return the same error when an entity
	// does not exist as for a bad password, so that
	// we don't allow unauthenticated users to find information
	// about existing entities.
	if err != nil || !entity.PasswordValid(password) {
		return errBadCreds
	}
	u.entity = entity
	return nil
}

// authenticator returns the currently logged-in authenticator entity, or nil
// if not currently logged on.  The returned entity should not be modified
// because it may be used concurrently.
func (u *authUser) authenticator() state.TaggedAuthenticator {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.entity
}
