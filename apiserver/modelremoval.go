// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"sync"

	coremodel "github.com/juju/juju/core/model"
)

// modelRemovals reports the removal of a model from this controller to the
// connections serving it. A model is removed when it is destroyed, or when a
// migration's REAP phase deletes it after migrating away; the connections
// serving the model must then be closed so that its agents and clients
// reconnect, dialling the addresses the migration wrote into their
// configuration.
//
// One channel is held per model being served, however many connections that
// is, and closing it reports the removal to all of them at once. The removals
// come from the single controller-wide watcher driven by [Server.loop], so a
// connection costs a map entry rather than a watcher of its own.
//
// Only the removal is reported here. Closing a connection is left to the
// goroutine serving it, which owns the connection's whole lifecycle: an
// rpc.Conn may not be closed before it has been started.
type modelRemovals struct {
	mu     sync.Mutex
	served map[coremodel.UUID]*servedModel
}

// servedModel is the removal state shared by every connection this API server
// is serving for one model.
type servedModel struct {
	// removed is closed once the model has been removed from the controller.
	removed chan struct{}

	// conns counts the connections being served for the model.
	conns int
}

func newModelRemovals() *modelRemovals {
	return &modelRemovals{
		served: make(map[coremodel.UUID]*servedModel),
	}
}

// track registers a connection being served for the given model. It returns a
// channel that is closed once the model has been removed from the controller,
// along with a function that must be called exactly once, when the connection
// is no longer being served.
//
// Callers must track the model before checking that it exists, so that a
// removal can never fall between the check and the registration.
func (r *modelRemovals) track(modelUUID coremodel.UUID) (<-chan struct{}, func()) {
	r.mu.Lock()
	defer r.mu.Unlock()

	served, ok := r.served[modelUUID]
	if !ok {
		served = &servedModel{removed: make(chan struct{})}
		r.served[modelUUID] = served
	}
	served.conns++

	return served.removed, func() { r.untrack(modelUUID, served) }
}

// untrack records that a connection is no longer being served for the model,
// forgetting the model once its last connection has gone.
func (r *modelRemovals) untrack(modelUUID coremodel.UUID, served *servedModel) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// A removed model is forgotten as it is reported, so anything held against
	// its UUID now belongs to later connections and is not ours to release.
	if r.served[modelUUID] != served {
		return
	}

	served.conns--
	if served.conns == 0 {
		delete(r.served, modelUUID)
	}
}

// notify reports the removal of the model to every connection being served for
// it, and forgets the model. Those connections keep the channel they were given
// and so are still woken, while a connection tracking the model afterwards can
// only be one answering a redirect, which must be served until its login is
// refused.
//
// Removals of models this API server is not serving are ignored.
func (r *modelRemovals) notify(modelUUID coremodel.UUID) {
	r.mu.Lock()
	defer r.mu.Unlock()

	served, ok := r.served[modelUUID]
	if !ok {
		return
	}
	delete(r.served, modelUUID)
	close(served.removed)
}
