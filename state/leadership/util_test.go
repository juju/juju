// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership_test

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/lease"
)

// Client implements lease.Client for testing purposes.
type Client struct {
	leases map[string]lease.Info
	expect []call
	failed string
	done   chan struct{}
}

// NewClient initializes and returns a new client configured to report
// the supplied leases and expect the supplied calls.
func NewClient(leases map[string]lease.Info, expect []call) *Client {
	if leases == nil {
		leases = make(map[string]lease.Info)
	}
	done := make(chan struct{})
	if len(expect) == 0 {
		close(done)
	}
	return &Client{
		leases: leases,
		expect: expect,
		done:   done,
	}
}

// Wait will return when all expected calls have been made, or fail the test
// if they don't happen within a second. (You control the clock; your tests
// should pass in *way* less than a second of wall-clock time.)
func (client *Client) Wait(c *gc.C) {
	select {
	case <-client.done:
		if client.failed != "" {
			c.Fatalf(client.failed)
		}
	case <-time.After(time.Second):
		c.Fatalf("Client test took way too long")
	}
}

// Leases is part of the lease.Client interface.
func (client *Client) Leases() map[string]lease.Info {
	result := make(map[string]lease.Info)
	for k, v := range client.leases {
		result[k] = v
	}
	return result
}

// call implements the bulk of the lease.Client interface.
func (client *Client) call(method string, args []interface{}) error {
	select {
	case <-client.done:
		return errors.Errorf("Client method called after test complete: %s %v", method, args)
	default:
		defer func() {
			if len(client.expect) == 0 || client.failed != "" {
				close(client.done)
			}
		}()
	}

	expect := client.expect[0]
	client.expect = client.expect[1:]
	if expect.callback != nil {
		expect.callback(client.leases)
	}

	if method == expect.method {
		if ok, _ := jc.DeepEqual(args, expect.args); ok {
			return expect.err
		}
	}
	client.failed = fmt.Sprintf("unexpected Client call:\n  actual: %s %v\n  expect: %s %v",
		method, args, expect.method, expect.args,
	)
	return errors.New(client.failed)
}

// ClaimLease is part of the lease.Client interface.
func (client *Client) ClaimLease(name string, request lease.Request) error {
	return client.call("ClaimLease", []interface{}{name, request})
}

// ExtendLease is part of the lease.Client interface.
func (client *Client) ExtendLease(name string, request lease.Request) error {
	return client.call("ExtendLease", []interface{}{name, request})
}

// ExpireLease is part of the lease.Client interface.
func (client *Client) ExpireLease(name string) error {
	return client.call("ExpireLease", []interface{}{name})
}

// Refresh is part of the lease.Client interface.
func (client *Client) Refresh() error {
	return client.call("Refresh", nil)
}

// call defines a expected method call on a Client; it encodes:
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
