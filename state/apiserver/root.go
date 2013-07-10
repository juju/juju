// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/apiserver/client"
	"launchpad.net/juju-core/state/apiserver/common"
	"launchpad.net/juju-core/state/apiserver/machine"
	"launchpad.net/juju-core/state/apiserver/upgrader"
	"launchpad.net/juju-core/state/multiwatcher"
)

type clientAPI struct{ *client.API }

// srvRoot represents a single client's connection to the state
// after it has logged in.
type srvRoot struct {
	clientAPI
	srv       *Server
	resources *common.Resources

	entity state.TaggedAuthenticator
}

func newSrvRoot(srv *Server, entity state.TaggedAuthenticator) *srvRoot {
	r := &srvRoot{
		srv:       srv,
		resources: common.NewResources(),
		entity:    entity,
	}
	r.clientAPI.API = client.NewAPI(srv.state, r.resources, r)
	return r
}

// Kill implements rpc.Killer.  It cleans up any resources that need
// cleaning up to ensure that all outstanding requests return.
func (r *srvRoot) Kill() {
	r.resources.StopAll()
}

// requireAgent checks whether the current client is an agent and hence
// may access the agent APIs.  We filter out non-agents when calling one
// of the accessor functions (Machine, Unit, etc) which avoids us making
// the check in every single request method.
func (r *srvRoot) requireAgent() error {
	if !isAgent(r.entity) {
		return common.ErrPerm
	}
	return nil
}

// requireClient returns an error unless the current
// client is a juju client user.
func (r *srvRoot) requireClient() error {
	if isAgent(r.entity) {
		return common.ErrPerm
	}
	return nil
}

// Machiner returns an object that provides access to the Machiner API
// facade. The id argument is reserved for future use and currently
// needs to be empty.
func (r *srvRoot) Machiner(id string) (*machine.MachinerAPI, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return nil, common.ErrBadId
	}
	return machine.NewMachinerAPI(r.srv.state, r.resources, r)
}

// MachineAgent returns an object that provides access to the machine
// agent API.  The id argument is reserved for future use and must currently
// be empty.
func (r *srvRoot) MachineAgent(id string) (*machine.AgentAPI, error) {
	if id != "" {
		return nil, common.ErrBadId
	}
	return machine.NewAgentAPI(r.srv.state, r)
}

// Upgrader returns an object that provides access to the Upgrader API facade.
// The id argument is reserved for future use and must be empty.
func (r *srvRoot) Upgrader(id string) (*upgrader.UpgraderAPI, error) {
	if id != "" {
		// TODO: There is no direct test for this
		return nil, common.ErrBadId
	}
	return upgrader.NewUpgraderAPI(r.srv.state, r.resources, r)
}

// NotifyWatcher returns an object that provides
// API access to methods on a state.NotifyWatcher.
// Each client has its own current set of watchers, stored
// in r.resources.
func (r *srvRoot) NotifyWatcher(id string) (*srvNotifyWatcher, error) {
	if err := r.requireAgent(); err != nil {
		return nil, err
	}
	watcher, ok := r.resources.Get(id).(state.NotifyWatcher)
	if !ok {
		return nil, common.ErrUnknownWatcher
	}
	return &srvNotifyWatcher{
		watcher:   watcher,
		id:        id,
		resources: r.resources,
	}, nil
}

// LifecycleWatcher returns an object that provides
// API access to methods on a state.LifecycleWatcher.
// Each client has its own current set of watchers, stored
// in r.resources.
func (r *srvRoot) LifecycleWatcher(id string) (*srvLifecycleWatcher, error) {
	if err := r.requireAgent(); err != nil {
		return nil, err
	}
	watcher, ok := r.resources.Get(id).(state.StringsWatcher)
	if !ok {
		return nil, common.ErrUnknownWatcher
	}
	return &srvLifecycleWatcher{
		watcher:   watcher,
		id:        id,
		resources: r.resources,
	}, nil
}

// AllWatcher returns an object that provides API access to methods on
// a state/multiwatcher.Watcher, which watches any changes to the
// state. Each client has its own current set of watchers, stored in
// r.resources.
func (r *srvRoot) AllWatcher(id string) (*srvClientAllWatcher, error) {
	if err := r.requireClient(); err != nil {
		return nil, err
	}
	watcher, ok := r.resources.Get(id).(*multiwatcher.Watcher)
	if !ok {
		return nil, common.ErrUnknownWatcher
	}
	return &srvClientAllWatcher{
		watcher:   watcher,
		id:        id,
		resources: r.resources,
	}, nil
}

// Pinger returns object with a single "Ping" method that does nothing.
func (r *srvRoot) Pinger(id string) (srvPinger, error) {
	return srvPinger{}, nil
}

type srvPinger struct{}

// Ping is a no-op used by client heartbeat monitor.
func (r srvPinger) Ping() {}

// AuthMachineAgent returns whether the current client is a machine agent.
func (r *srvRoot) AuthMachineAgent() bool {
	_, ok := r.entity.(*state.Machine)
	return ok
}

// AuthOwner returns whether the authenticated user's tag matches the
// given entity tag.
func (r *srvRoot) AuthOwner(tag string) bool {
	return r.entity.Tag() == tag
}

// AuthEnvironManager returns whether the authenticated user is a
// machine with running the ManageEnviron job.
func (r *srvRoot) AuthEnvironManager() bool {
	return isMachineWithJob(r.entity, state.JobManageEnviron)
}

func (r *srvRoot) AuthClient() bool {
	return !isAgent(r.entity)
}
