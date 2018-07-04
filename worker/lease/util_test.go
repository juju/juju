// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/lease"
	coretesting "github.com/juju/juju/testing"
)

// Secretary implements lease.Secretary for testing purposes.
type Secretary struct{}

// CheckLease is part of the lease.Secretary interface.
func (Secretary) CheckLease(name string) error {
	return checkName(name)
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
	leases map[string]lease.Info
	expect []call
	failed string
	done   chan struct{}
}

// NewStore initializes and returns a new store configured to report
// the supplied leases and expect the supplied calls.
func NewStore(leases map[string]lease.Info, expect []call) *Store {
	if leases == nil {
		leases = make(map[string]lease.Info)
	}
	done := make(chan struct{})
	if len(expect) == 0 {
		close(done)
	}
	return &Store{
		leases: leases,
		expect: expect,
		done:   done,
	}
}

// Wait will return when all expected calls have been made, or fail the test
// if they don't happen within a second. (You control the clock; your tests
// should pass in *way* less than 10 seconds of wall-clock time.)
func (store *Store) Wait(c *gc.C) {
	select {
	case <-store.done:
		if store.failed != "" {
			c.Fatalf(store.failed)
		}
	case <-time.After(coretesting.LongWait):
		c.Fatalf("Store test took way too long")
	}
}

// Leases is part of the lease.Store interface.
func (store *Store) Leases() map[lease.Key]lease.Info {
	result := make(map[lease.Key]lease.Info)
	for k, v := range store.leases {
		result[lease.Key{Lease: k}] = v
	}
	return result
}

// call implements the bulk of the lease.Store interface.
func (store *Store) call(method string, args []interface{}) error {
	select {
	case <-store.done:
		return errors.Errorf("Store method called after test complete: %s %v", method, args)
	default:
		defer func() {
			if len(store.expect) == 0 || store.failed != "" {
				close(store.done)
			}
		}()
	}

	expect := store.expect[0]
	store.expect = store.expect[1:]
	if expect.callback != nil {
		expect.callback(store.leases)
	}

	if method == expect.method {
		if ok, _ := jc.DeepEqual(args, expect.args); ok {
			return expect.err
		}
	}
	store.failed = fmt.Sprintf("unexpected Store call:\n  actual: %s %v\n  expect: %s %v",
		method, args, expect.method, expect.args,
	)
	return errors.New(store.failed)
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
	callback func(leases map[string]lease.Info)
}
