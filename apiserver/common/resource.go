// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"
	"strconv"
	"sync"

	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"
)

// Resources holds all the resources for a connection.
// It allows the registration of resources that will be cleaned
// up when a connection terminates.
type Resources struct {
	mu        sync.Mutex
	maxId     uint64
	resources map[string]worker.Worker

	// The stack is used to control the order of destruction.
	// last registered, first stopped.
	// XXX(fwereade): is this necessary only because we have
	// Resource instead of Worker (which would let us kill them all,
	// and wait for them all, without danger of races)?
	stack []string
}

func NewResources() *Resources {
	return &Resources{
		resources: make(map[string]worker.Worker),
	}
}

// Get returns the resource for the given id, or
// nil if there is no such resource.
func (rs *Resources) Get(id string) worker.Worker {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.resources[id]
}

// Register registers the given resource. It returns a unique
// identifier for the resource which can then be used in
// subsequent API requests to refer to the resource.
func (rs *Resources) Register(r worker.Worker) string {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.maxId++
	id := strconv.FormatUint(rs.maxId, 10)
	rs.resources[id] = r
	rs.stack = append(rs.stack, id)
	logger.Tracef("registered unnamed resource: %s", id)
	return id
}

// RegisterNamed registers the given resource. Callers must supply a unique
// name for the given resource. It is an error to try to register another
// resource with the same name as an already registered name. (This could be
// softened that you can overwrite an existing one and it will be Stopped and
// replaced, but we don't have a need for that yet.)
// It is also an error to supply a name that is an integer string, since that
// collides with the auto-naming from Register.
func (rs *Resources) RegisterNamed(name string, r worker.Worker) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if _, err := strconv.Atoi(name); err == nil {
		return fmt.Errorf("RegisterNamed does not allow integer names: %q", name)
	}
	if _, ok := rs.resources[name]; ok {
		return fmt.Errorf("resource %q already registered", name)
	}
	rs.resources[name] = r
	rs.stack = append(rs.stack, name)
	logger.Tracef("registered named resource: %s", name)
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
	logger.Tracef("stopping resource: %s", id)
	r := rs.Get(id)
	if r == nil {
		return nil
	}
	rs.mu.Lock()
	defer rs.mu.Unlock()
	r.Kill()
	err := r.Wait()
	delete(rs.resources, id)
	for pos := 0; pos < len(rs.stack); pos++ {
		if rs.stack[pos] == id {
			rs.stack = append(rs.stack[0:pos], rs.stack[pos+1:]...)
			break
		}
	}
	return err
}

// StopAll stops all the resources.
func (rs *Resources) StopAll() {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	for i := len(rs.stack); i > 0; i-- {
		id := rs.stack[i-1]
		r := rs.resources[id]
		logger.Tracef("stopping resource: %s", id)
		r.Kill()
		if err := r.Wait(); err != nil {
			logger.Errorf("error stopping %T resource: %v", r, err)
		}
	}
	rs.resources = make(map[string]worker.Worker)
	rs.stack = nil
}

// Count returns the number of resources currently held.
func (rs *Resources) Count() int {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return len(rs.resources)
}

// FIXME(nvinuesa): This `ValueResource` should be removed and they should not
// be registered.
// ValueResource is a Resource with a no-op Stop method, containing any value.
// It does nothing but waits for heat death of the universe, whilst holding
// a value.
type ValueResource struct {
	tomb  tomb.Tomb
	Value any
}

func NewValueResource(value any) *ValueResource {
	res := &ValueResource{
		Value: value,
	}
	res.tomb.Go(func() error {
		<-res.tomb.Dying()
		return tomb.ErrDying
	})
	return res
}

func (v *ValueResource) Kill() {
	v.tomb.Kill(nil)
}

func (v *ValueResource) Wait() error {
	return v.tomb.Wait()
}
