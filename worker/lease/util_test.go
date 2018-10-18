// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"sync"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/core/lease"
	coretesting "github.com/juju/juju/testing"
)

// Secretary implements lease.Secretary for testing purposes.
type Secretary struct{}

// CheckLease is part of the lease.Secretary interface.
func (Secretary) CheckLease(key lease.Key) error {
	return checkName(key.Lease)
}

// CheckHolder is part of the lease.Secretary interface.
func (Secretary) CheckHolder(name string) error {
	return checkName(name)
}

func checkName(name string) error {
	if name == "INVALID" {
		return errors.NotValidf("name")
	}
	return nil
}

// CheckDuration is part of the lease.Secretary interface.
func (Secretary) CheckDuration(duration time.Duration) error {
	if duration != time.Minute {
		return errors.NotValidf("time")
	}
	return nil
}

// Store implements corelease.Store for testing purposes.
type Store struct {
	mu           sync.Mutex
	leases       map[lease.Key]lease.Info
	expect       []call
	failed       chan error
	runningCalls int
	done         chan struct{}
}

// NewStore initializes and returns a new store configured to report
// the supplied leases and expect the supplied calls.
func NewStore(leases map[lease.Key]lease.Info, expect []call) *Store {
	if leases == nil {
		leases = make(map[lease.Key]lease.Info)
	}
	done := make(chan struct{})
	if len(expect) == 0 {
		close(done)
	}
	return &Store{
		leases: leases,
		expect: expect,
		done:   done,
		failed: make(chan error, 1000),
	}
}

// Wait will return when all expected calls have been made, or fail the test
// if they don't happen within a second. (You control the clock; your tests
// should pass in *way* less than 10 seconds of wall-clock time.)
func (store *Store) Wait(c *gc.C) {
	select {
	case <-store.done:
		select {
		case err := <-store.failed:
			c.Fatalf(err.Error())
		default:
		}
	case <-time.After(coretesting.LongWait):
		c.Fatalf("Store test took way too long")
	}
}

// Leases is part of the lease.Store interface.
func (store *Store) Leases() map[lease.Key]lease.Info {
	store.mu.Lock()
	defer store.mu.Unlock()
	result := make(map[lease.Key]lease.Info)
	for k, v := range store.leases {
		result[k] = v
	}
	return result
}

func (store *Store) closeIfEmpty() {
	// This must be called with the lock held.
	if store.runningCalls > 1 {
		// The last one to leave should turn out the lights.
		return
	}
	if len(store.expect) == 0 || len(store.failed) > 0 {
		close(store.done)
	}
}

// call implements the bulk of the lease.Store interface.
func (store *Store) call(method string, args []interface{}) error {
	store.mu.Lock()
	defer store.mu.Unlock()

	store.runningCalls++
	defer func() {
		store.runningCalls--
	}()

	select {
	case <-store.done:
		err := errors.Errorf("Store method called after test complete: %s %v", method, args)
		store.failed <- err
		return err
	default:
	}
	defer store.closeIfEmpty()

	expect := store.expect[0]
	store.expect = store.expect[1:]
	if expect.parallelCallback != nil {
		store.mu.Unlock()
		expect.parallelCallback(&store.mu, store.leases)
		store.mu.Lock()
	}
	if expect.callback != nil {
		expect.callback(store.leases)
	}

	if method == expect.method {
		if ok, _ := jc.DeepEqual(args, expect.args); ok {
			return expect.err
		}
	}
	err := errors.Errorf("unexpected Store call:\n  actual: %s %v\n  expect: %s %v",
		method, args, expect.method, expect.args,
	)
	store.failed <- err
	return err
}

// ClaimLease is part of the corelease.Store interface.
func (store *Store) ClaimLease(key lease.Key, request lease.Request) error {
	return store.call("ClaimLease", []interface{}{key, request})
}

// ExtendLease is part of the corelease.Store interface.
func (store *Store) ExtendLease(key lease.Key, request lease.Request) error {
	return store.call("ExtendLease", []interface{}{key, request})
}

// ExpireLease is part of the corelease.Store interface.
func (store *Store) ExpireLease(key lease.Key) error {
	return store.call("ExpireLease", []interface{}{key})
}

// Refresh is part of the lease.Store interface.
func (store *Store) Refresh() error {
	return store.call("Refresh", nil)
}

// PinLease is part of the corelease.Store interface.
func (store *Store) PinLease(key lease.Key, entity names.Tag) error {
	return store.call("PinLease", []interface{}{key, entity})
}

// UnpinLease is part of the corelease.Store interface.
func (store *Store) UnpinLease(key lease.Key, entity names.Tag) error {
	return store.call("UnpinLease", []interface{}{key, entity})
}

// call defines a expected method call on a Store; it encodes:
type call struct {

	// method is the name of the method.
	method string

	// args is the expected arguments.
	args []interface{}

	// err is the error to return.
	err error

	// callback, if non-nil, will be passed the internal leases dict; for
	// modification, if desired. Otherwise you can use it to, e.g., assert
	// clock time.
	callback func(leases map[lease.Key]lease.Info)

	// parallelCallback is like callback, but is also passed the
	// lock. It's for testing calls that happen in parallel, where one
	// might take longer than another. Any update to the leases dict
	// must only happen while the lock is held.
	parallelCallback func(mu *sync.Mutex, leases map[lease.Key]lease.Info)
}

func key(args ...string) lease.Key {
	result := lease.Key{
		Namespace: "namespace",
		ModelUUID: "modelUUID",
		Lease:     "lease",
	}
	if len(args) == 1 {
		result.Lease = args[0]
	} else if len(args) == 3 {
		result.Namespace = args[0]
		result.ModelUUID = args[1]
		result.Lease = args[2]
	}
	return result
}
