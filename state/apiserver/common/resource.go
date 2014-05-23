// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"
	"strconv"
	"sync"
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

// RegisterNamed registers the given resource. Callers must supply a unique
// name for the given resource. It is an error to try to register another
// resource with the same name as an already registered name. (This could be
// softened that you can overwrite an existing one and it will be Stopped and
// replaced, but we don't have a need for that yet.)
// It is also an error to supply a name that is an integer string, since that
// collides with the auto-naming from Register.
func (rs *Resources) RegisterNamed(name string, r Resource) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if _, err := strconv.Atoi(name); err == nil {
		return fmt.Errorf("RegisterNamed does not allow integer names: %q", name)
	}
	if _, ok := rs.resources[name]; ok {
		return fmt.Errorf("resource %q already registered", name)
	}
	rs.resources[name] = r
	return nil
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
			logger.Errorf("error stopping %T resource: %v", r, err)
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

// StringResource is just a regular 'string' that matches the Resource
// interface.
type StringResource string

func (StringResource) Stop() error {
	return nil
}

func (s StringResource) String() string {
	return string(s)
}
