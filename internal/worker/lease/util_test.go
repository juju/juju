// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"context"
	"sync"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/core/lease"
	coretesting "github.com/juju/juju/internal/testing"
)

// SecretaryFinder implements lease.SecretaryFinder for testing purposes.
type SecretaryFinder struct {
	fn func(string) (lease.Secretary, error)
}

// FuncSecretaryFinder returns a SecretaryFinder that calls the supplied
// function to find the Secretary.
func FuncSecretaryFinder(fn func(string) (lease.Secretary, error)) SecretaryFinder {
	return SecretaryFinder{fn: fn}
}

// Register adds a Secretary to the Cabinet.
func (c SecretaryFinder) Register(namespace string, secretary lease.Secretary) {}

// SecretaryFor returns the Secretary for the given namespace.
// Returns an error if the namespace is not valid.
func (c SecretaryFinder) SecretaryFor(namespace string) (lease.Secretary, error) {
	return c.fn(namespace)
}

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
	clock        *testclock.Clock
	leases       map[lease.Key]lease.Info
	expect       []call
	failed       chan error
	runningCalls int
	done         chan struct{}
}

// NewStore initializes and returns a new store configured to report
// the supplied leases and expect the supplied calls.
func NewStore(leases map[lease.Key]lease.Info, expect []call, clock *testclock.Clock) *Store {
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
		clock:  clock,
	}
}

// Wait will return when all expected calls have been made, or fail the test
// if they don't happen within a second. (You control the clock; your tests
// should pass in *way* less than 10 seconds of wall-clock time.)
func (store *Store) Wait(c *tc.C) {
	select {
	case <-store.done:
		select {
		case err := <-store.failed:
			c.Errorf(err.Error())
		default:
		}
	case <-time.After(coretesting.LongWait):
		store.mu.Lock()
		remaining := make([]string, len(store.expect))
		for i := range store.expect {
			remaining[i] = store.expect[i].method
		}
		store.mu.Unlock()
		c.Errorf("Store test took way too long, still expecting %v", remaining)
	}
}

func (store *Store) expireLeases() {
	store.mu.Lock()
	defer store.mu.Unlock()
	for k, v := range store.leases {
		if store.clock.Now().Before(v.Expiry) {
			continue
		}
		delete(store.leases, k)
	}
}

// Leases is part of the lease.Store interface.
func (store *Store) Leases(_ context.Context, keys ...lease.Key) (map[lease.Key]lease.Info, error) {
	filter := make(map[lease.Key]bool)
	filtering := len(keys) > 0
	if filtering {
		for _, key := range keys {
			filter[key] = true
		}
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	result := make(map[lease.Key]lease.Info)
	for k, v := range store.leases {
		if filtering && !filter[k] {
			continue
		}
		result[k] = v
	}
	return result, nil
}

// LeaseGroup is part of the lease.Store interface.
func (store *Store) LeaseGroup(ctx context.Context, namespace, modelUUID string) (map[lease.Key]lease.Info, error) {
	leases, err := store.Leases(ctx)
	if err != nil {
		return nil, err
	}

	results := make(map[lease.Key]lease.Info)
	for key, info := range leases {
		if key.Namespace == namespace && key.ModelUUID == modelUUID {
			results[key] = info
		}
	}
	return results, nil
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

	if len(store.expect) < 1 {
		err := errors.Errorf("store.%s called but was not expected", method)
		store.failed <- err
		return err
	}
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
		if ok, _ := tc.DeepEqual(args, expect.args); ok {
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
func (store *Store) ClaimLease(_ context.Context, key lease.Key, request lease.Request) error {
	return store.call("ClaimLease", []interface{}{key, request})
}

// ExtendLease is part of the corelease.Store interface.
func (store *Store) ExtendLease(_ context.Context, key lease.Key, request lease.Request) error {
	return store.call("ExtendLease", []interface{}{key, request})
}

func (store *Store) RevokeLease(_ context.Context, lease lease.Key, holder string) error {
	return store.call("RevokeLease", []interface{}{lease, holder})
}

// PinLease is part of the corelease.Store interface.
func (store *Store) PinLease(_ context.Context, key lease.Key, entity string) error {
	return store.call("PinLease", []interface{}{key, entity})
}

// UnpinLease is part of the corelease.Store interface.
func (store *Store) UnpinLease(_ context.Context, key lease.Key, entity string) error {
	return store.call("UnpinLease", []interface{}{key, entity})
}

func (store *Store) Pinned(_ context.Context) (map[lease.Key][]string, error) {
	_ = store.call("Pinned", nil)
	return map[lease.Key][]string{
		{
			Namespace: "namespace",
			ModelUUID: "modelUUID",
			Lease:     "redis",
		}: {names.NewMachineTag("0").String()},
		{
			Namespace: "ignored-namespace",
			ModelUUID: "ignored modelUUID",
			Lease:     "lolwut",
		}: {names.NewMachineTag("666").String()},
	}, nil
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
