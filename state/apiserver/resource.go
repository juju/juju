// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"launchpad.net/juju-core/log"
	"strconv"
	"sync"
)

// resource represents the interface provided by state watchers and pingers.
type resource interface {
	Stop() error
}

// resources holds all the resources for a connection.
type resources struct {
	mu    sync.Mutex
	maxId uint64
	rs    map[string]*srvResource
}

// srvResource holds the details of a resource. It also implements the
// Stop RPC method for all resources.
type srvResource struct {
	rs       *resources
	resource resource
	id       string
}

// Stop stops the given resource. It causes any outstanding
// Next calls to return a CodeStopped error.
// Any subsequent Next calls will return a CodeNotFound
// error because the resource will no longer exist.
func (r *srvResource) Stop() error {
	err := r.resource.Stop()
	r.rs.mu.Lock()
	defer r.rs.mu.Unlock()
	delete(r.rs.rs, r.id)
	return err
}

func newResources() *resources {
	return &resources{
		rs: make(map[string]*srvResource),
	}
}

// get returns the srvResource registered with the given
// id, or nil if there is no such resource.
func (rs *resources) get(id string) *srvResource {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.rs[id]
}

// register records the given watcher and returns
// a srvResource instance for it.
func (rs *resources) register(r resource) *srvResource {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.maxId++
	sr := &srvResource{
		rs:       rs,
		id:       strconv.FormatUint(rs.maxId, 10),
		resource: r,
	}
	rs.rs[sr.id] = sr
	return sr
}

func (rs *resources) stopAll() {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	for _, r := range rs.rs {
		if err := r.resource.Stop(); err != nil {
			log.Errorf("state/api: error stopping %T resource: %v", r, err)
		}
	}
	rs.rs = make(map[string]*srvResource)
}
