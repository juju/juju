// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"launchpad.net/juju-core/state/api/params"
	statewatcher "launchpad.net/juju-core/state/watcher"
)

// srvState serves agent-specific top-level state API methods.
type srvState struct {
	root *srvRoot
}

// AllMachines returns all machines in the environment ordered by id.
func (s *srvState) AllMachines() (params.AllMachinesResults, error) {
	if !s.root.authEnvironManager() {
		return params.AllMachinesResults{}, errPerm
	}
	machines, err := s.root.srv.state.AllMachines()
	if err != nil {
		return params.AllMachinesResults{}, err
	}
	results := params.AllMachinesResults{
		Machines: make([]*params.Machine, len(machines)),
	}
	for i, m := range machines {
		results.Machines[i] = stateMachineToParams(m)
	}
	return results, nil
}

// WatchMachines registers a srvLifecycleWatcher that notifies of
// changes to the lifecycles of the machines in the environment. The
// result contains the id of the registered watcher and the initial
// list of machine ids.
func (s *srvState) WatchMachines() (params.LifecycleWatchResults, error) {
	if !s.root.authEnvironManager() {
		return params.LifecycleWatchResults{}, errPerm
	}
	watcher := s.root.srv.state.WatchMachines()
	// The watcher always sends an initial value on the channel,
	// so we send that as the result of the watch request.
	// This saves the client a round trip.
	initial, ok := <-watcher.Changes()
	if !ok {
		return params.LifecycleWatchResults{}, statewatcher.MustErr(watcher)
	}
	return params.LifecycleWatchResults{
		LifecycleWatcherId: s.root.resources.register(watcher).id,
		Ids:                initial,
	}, nil
}

// WatchEnvironConfig registers a srvEnvironConfigWatcher for
// observing changes to the environment configuration. The result
// contains the id of the registered watcher and the current
// environment configuration.
func (s *srvState) WatchEnvironConfig() (params.EnvironConfigWatchResults, error) {
	if !s.root.authEnvironManager() {
		return params.EnvironConfigWatchResults{}, errPerm
	}
	watcher := s.root.srv.state.WatchEnvironConfig()
	// The watcher always sends an initial value on the channel,
	// so we send that as the result of the watch request.
	// This saves the client a round trip.
	initial, ok := <-watcher.Changes()
	if !ok {
		return params.EnvironConfigWatchResults{}, statewatcher.MustErr(watcher)
	}
	return params.EnvironConfigWatchResults{
		EnvironConfigWatcherId: s.root.resources.register(watcher).id,
		Config:                 initial.AllAttrs(),
	}, nil
}
