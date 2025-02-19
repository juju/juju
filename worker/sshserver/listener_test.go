// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver_test

import (
	"sync"

	"github.com/juju/testing"
	"google.golang.org/grpc/test/bufconn"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/sshserver"
)

type listenerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&listenerSuite{})

func (s *listenerSuite) TestAcceptOnceListener(c *gc.C) {
	listener := bufconn.Listen(8 * 1024)

	acceptOnceListener, closeAllowed := sshserver.NewAcceptOnceListener(listener)
	c.Assert(acceptOnceListener, gc.NotNil)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		<-closeAllowed
		listener.Close()
	}()

	go func() {
		defer wg.Done()
		// Accept runs and sends down the channel, it is blocked then until
		// Close continues.
		acceptOnceListener.Accept()
	}()

	wg.Wait()
}
