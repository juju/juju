// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/utils/fslock"
	"launchpad.net/juju-core/worker/uniter"
)

type ListenerSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&ListenerSuite{})

// Mirror the params to uniter.NewRunListener, but add cleanup to close it.
func (s *ListenerSuite) NewRunListener(hookLock *fslock.Lock, netType, localAddr string) (*uniter.RunListener, error) {
	listener, err := uniter.NewRunListener(hookLock, netType, localAddr)
	if listener != nil {
		go listener.Run()
		s.AddCleanup(func(*gc.C) {
			listener.Close()
		})
	}
	return listener, err
}

func (s *ListenerSuite) TestNewRunListener(c *gc.C) {
	// TODO: be nicer about fslock param
	socketPath := "/tmp/test.listener"
	listener, err := s.NewRunListener(nil, "unix", socketPath)
	c.Assert(err, gc.IsNil)
	c.Assert(listener, gc.NotNil)
}

func (s *ListenerSuite) TestNewRunListenerSecondFails(c *gc.C) {
	// TODO: be nicer about fslock param
	socketPath := "/tmp/test.listener"
	_, err := s.NewRunListener(nil, "unix", socketPath)
	c.Assert(err, gc.IsNil)

	listener, err := s.NewRunListener(nil, "unix", socketPath)

	c.Assert(listener, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, ".* address already in use")
}
