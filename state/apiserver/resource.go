// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state/apiserver/common"
	"strconv"
	"sync"
)

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
	resource common.Resource
	id       string
}

func newResources() *resources {
	return &resources{
		rs: make(map[string]*srvResource),
	}
}

// Get implements common.ResourceRegistry.Get.
func (rs *resources) Get(id string) common.Resource {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if r := rs.rs[id]; r != nil {
		return r.resource
	}
	return nil
}

// Register implements common.ResourceRegistry.Register.
func (rs *resources) Register(r common.Resource) string {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.maxId++
	sr := &srvResource{
		rs:       rs,
		id:       strconv.FormatUint(rs.maxId, 10),
		resource: r,
	}
	rs.rs[sr.id] = sr
	return sr.id
}

// Stop implements common.ResourceRegistry.Stop.
func (rs *resources) Stop(id string) error {
	// We don't hold the mutex while calling Stop, because
	// that might take a while and we don't want to
	// stop all other resource manipulation while we do so.
	// If resources.Stop is called concurrently, we'll get
	// two concurrent calls to Stop, but that should fit
	// well with the way we invariably implement Stop.
	r := rs.Get(id)
	if r == nil {
		return nil
	}
	err := r.Stop()
	rs.mu.Lock()
	defer rs.mu.Unlock()
	delete(rs.rs, id)
	return err
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
