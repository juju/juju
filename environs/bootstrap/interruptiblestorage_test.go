// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap_test

import (
	"fmt"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/bootstrap"
	envtesting "launchpad.net/juju-core/environs/testing"
	coretesting "launchpad.net/juju-core/testing"
)

type interruptibleStorageSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&interruptibleStorageSuite{})

type errorReader struct {
	close  chan struct{}
	wait   chan struct{}
	called int
	err    error
}

func (r *errorReader) Read(buf []byte) (int, error) {
	if r.close != nil {
		close(r.close)
	}
	if r.wait != nil {
		<-r.wait
	}
	r.called++
	return 0, r.err
}

func (s *interruptibleStorageSuite) TestInterruptStorage(c *gc.C) {
	closer, stor, _ := envtesting.CreateLocalTestStorage(c)
	s.AddCleanup(func(c *gc.C) { closer.Close() })
	reader := &errorReader{
		err: fmt.Errorf("read failed"),
	}
	interrupted := make(chan struct{})
	istor := bootstrap.NewInterruptibleStorage(stor, interrupted)

	err := istor.Put("name", reader, 3)
	c.Assert(err, gc.ErrorMatches, ".*: read failed")
	c.Assert(reader.called, gc.Equals, 1)

	// If the channel is already closed, then the
	// underlying reader is never deferred to.
	close(interrupted)
	err = istor.Put("name", reader, 3)
	c.Assert(err, gc.ErrorMatches, ".*: interrupted")
	c.Assert(reader.called, gc.Equals, 1)
}

func (s *interruptibleStorageSuite) TestInterruptStorageConcurrently(c *gc.C) {
	closer, stor, _ := envtesting.CreateLocalTestStorage(c)
	s.AddCleanup(func(c *gc.C) { closer.Close() })
	reader := &errorReader{
		close: make(chan struct{}),
		wait:  make(chan struct{}),
		err:   fmt.Errorf("read failed"),
	}
	istor := bootstrap.NewInterruptibleStorage(stor, reader.close)
	err := istor.Put("name", reader, 3)
	c.Assert(err, gc.ErrorMatches, ".*: interrupted")
	c.Assert(reader.called, gc.Equals, 0) // reader is blocked
	close(reader.wait)
}
