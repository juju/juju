// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"launchpad.net/juju-core/rpc"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/apiserver/common"
	"launchpad.net/juju-core/state/apiserver/machiner"
	"launchpad.net/juju-core/state/multiwatcher"
)

// srvRoot represents a single client's connection to the state.
type srvRoot struct {
	admin     *srvAdmin
	client    *srvClient
	state     *srvState
	srv       *Server
	resources *resources
	rpcConn   *rpc.Conn

	user authUser
}

func newStateServer(srv *Server, rpcConn *rpc.Conn) *srvRoot {
	r := &srvRoot{
		srv:       srv,
		resources: newResources(),
		rpcConn:   rpcConn,
	}
	r.admin = &srvAdmin{
		root: r,
	}
	r.client = &srvClient{
		root: r,
	}
	r.state = &srvState{
		root: r,
	}
	return r
}

// Kill implements rpc.Killer.  It cleans up any resources that need
// cleaning up to ensure that all outstanding requests return.
func (r *srvRoot) Kill() {
	r.resources.stopAll()
}

// Admin returns an object that provides API access
// to methods that can be called even when not
// authenticated.
func (r *srvRoot) Admin(id string) (*srvAdmin, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return nil, common.ErrBadId
	}
	return r.admin, nil
}

// requireAgent checks whether the current client is an agent and hence
// may access the agent APIs.  We filter out non-agents when calling one
// of the accessor functions (Machine, Unit, etc) which avoids us making
// the check in every single request method.
func (r *srvRoot) requireAgent() error {
	e := r.user.authenticator()
	if e == nil {
		return common.ErrNotLoggedIn
	}
	if !isAgent(e) {
		return common.ErrPerm
	}
	return nil
}

// requireClient returns an error unless the current
// client is a juju client user.
func (r *srvRoot) requireClient() error {
	e := r.user.authenticator()
	if e == nil {
		return common.ErrNotLoggedIn
	}
	if isAgent(e) {
		return common.ErrPerm
	}
	return nil
}

// Machiner returns an object that provides access to the Machiner API
// facade. The id argument is reserved for future use and currently
// needs to be empty.
func (r *srvRoot) Machiner(id string) (*machiner.Machiner, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return nil, common.ErrBadId
	}
	return machiner.New(r.srv.state, r)
}

// User returns an object that provides
// API access to methods on a state.User.
func (r *srvRoot) User(name string) (*srvUser, error) {
	// Any user is allowed to access their own user object.
	// We check at this level rather than at the operation
	// level to stop malicious probing for current user names.
	// When we provide support for user administration,
	// this will need to be changed to allow access to
	// the administrator.
	e := r.user.authenticator()
	if e == nil {
		return nil, common.ErrNotLoggedIn
	}
	if e.Tag() != name {
		return nil, common.ErrPerm
	}
	u, err := r.srv.state.User(name)
	if err != nil {
		return nil, err
	}
	return &srvUser{
		root: r,
		u:    u,
	}, nil
}

// EntityWatcher returns an object that provides
// API access to methods on a state.EntityWatcher.
// Each client has its own current set of watchers, stored
// in r.resources.
func (r *srvRoot) EntityWatcher(id string) (srvEntityWatcher, error) {
	if err := r.requireAgent(); err != nil {
		return srvEntityWatcher{}, err
	}
	watcher := r.resources.get(id)
	if watcher == nil {
		return srvEntityWatcher{}, common.ErrUnknownWatcher
	}
	if _, ok := watcher.resource.(*state.EntityWatcher); !ok {
		return srvEntityWatcher{}, common.ErrUnknownWatcher
	}
	return srvEntityWatcher{watcher}, nil
}

// LifecycleWatcher returns an object that provides
// API access to methods on a state.LifecycleWatcher.
// Each client has its own current set of watchers, stored
// in r.resources.
func (r *srvRoot) LifecycleWatcher(id string) (srvLifecycleWatcher, error) {
	if err := r.requireAgent(); err != nil {
		return srvLifecycleWatcher{}, err
	}
	watcher := r.resources.get(id)
	if watcher == nil {
		return srvLifecycleWatcher{}, common.ErrUnknownWatcher
	}
	if _, ok := watcher.resource.(*state.LifecycleWatcher); !ok {
		return srvLifecycleWatcher{}, common.ErrUnknownWatcher
	}
	return srvLifecycleWatcher{watcher}, nil
}

// EnvironConfigWatcher returns an object that provides
// API access to methods on a state.EnvironConfigWatcher.
// Each client has its own current set of watchers, stored
// in r.resources.
func (r *srvRoot) EnvironConfigWatcher(id string) (srvEnvironConfigWatcher, error) {
	if err := r.requireAgent(); err != nil {
		return srvEnvironConfigWatcher{}, err
	}
	watcher := r.resources.get(id)
	if watcher == nil {
		return srvEnvironConfigWatcher{}, common.ErrUnknownWatcher
	}
	if _, ok := watcher.resource.(*state.EnvironConfigWatcher); !ok {
		return srvEnvironConfigWatcher{}, common.ErrUnknownWatcher
	}
	return srvEnvironConfigWatcher{watcher}, nil
}

// AllWatcher returns an object that provides API access to methods on
// a state/multiwatcher.Watcher, which watches any changes to the
// state. Each client has its own current set of watchers, stored in
// r.resources.
func (r *srvRoot) AllWatcher(id string) (srvClientAllWatcher, error) {
	if err := r.requireClient(); err != nil {
		return srvClientAllWatcher{}, err
	}
	watcher := r.resources.get(id)
	if watcher == nil {
		return srvClientAllWatcher{}, common.ErrUnknownWatcher
	}
	if _, ok := watcher.resource.(*multiwatcher.Watcher); !ok {
		return srvClientAllWatcher{}, common.ErrUnknownWatcher
	}
	return srvClientAllWatcher{watcher}, nil

}

// State returns an object that provides API access to top-level state methods.
func (r *srvRoot) State(id string) (*srvState, error) {
	if err := r.requireAgent(); err != nil {
		return nil, err
	}
	if id != "" {
		// Safeguard id for possible future use.
		return nil, common.ErrBadId
	}
	return r.state, nil
}

// Client returns an object that provides access
// to methods accessible to non-agent clients.
func (r *srvRoot) Client(id string) (*srvClient, error) {
	if err := r.requireClient(); err != nil {
		return nil, err
	}
	if id != "" {
		// Safeguard id for possible future use.
		return nil, common.ErrBadId
	}
	return r.client, nil
}

// IsLoggedIn returns whether the user is currently logged in and
// authenticated.
func (r *srvRoot) IsLoggedIn() bool {
	return r.user.authenticator() != nil
}

// AuthMachineAgent returns whether the current client is a machine agent.
func (r *srvRoot) AuthMachineAgent() bool {
	if !r.IsLoggedIn() {
		return false
	}
	e := r.user.authenticator()
	if _, ok := e.(*state.Machine); !ok {
		return false
	}
	return true
}

// AuthOwner returns whether the authenticated user's tag matches the
// given entity's tag.
func (r *srvRoot) AuthOwner(entity common.Tagger) bool {
	authUser := r.user.authenticator()
	return authUser.Tag() == entity.Tag()
}

// AuthEnvironManager returns whether the authenticated user is a
// machine with running the ManageEnviron job.
func (r *srvRoot) AuthEnvironManager() bool {
	authUser := r.user.authenticator()
	return isMachineWithJob(authUser, state.JobManageEnviron)
}
