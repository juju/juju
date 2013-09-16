// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"strconv"
	"sync"

	"launchpad.net/juju-core/log"
)

// Resource represents any resource that should be cleaned up when an
// API connection terminates. The Stop method will be called when
// that happens.
type Resource interface {
	Stop() error
}

// Resources holds all the resources for a connection.
// It allows the registration of resources that will be cleaned
// up when a connection terminates.
type Resources struct {
	mu        sync.Mutex
	maxId     uint64
	resources map[string]Resource
}

func NewResources() *Resources {
	return &Resources{
		resources: make(map[string]Resource),
	}
}

// Get returns the resource for the given id, or
// nil if there is no such resource.
func (rs *Resources) Get(id string) Resource {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.resources[id]
}

// Register registers the given resource. It returns a unique
// identifier for the resource which can then be used in
// subsequent API requests to refer to the resource.
func (rs *Resources) Register(r Resource) string {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.maxId++
	id := strconv.FormatUint(rs.maxId, 10)
	rs.resources[id] = r
	return id
}

// Stop stops the resource with the given id and unregisters it.
// It returns any error from the underlying Stop call.
// It does not return an error if the resource has already
// been unregistered.
func (rs *Resources) Stop(id string) error {
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
	delete(rs.resources, id)
	return err
}

// StopAll stops all the resources.
func (rs *Resources) StopAll() {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	for _, r := range rs.resources {
		if err := r.Stop(); err != nil {
			log.Errorf("state/api: error stopping %T resource: %v", r, err)
		}
	}
	rs.resources = make(map[string]Resource)
}

// Count returns the number of resources currently held.
func (rs *Resources) Count() int {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return len(rs.resources)
}
