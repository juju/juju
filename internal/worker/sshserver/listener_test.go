// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver_test

import (
	"time"

	"github.com/juju/testing"
	"google.golang.org/grpc/test/bufconn"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/worker/sshserver"
)

type listenerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&listenerSuite{})

func (s *listenerSuite) TestAcceptOnceListener(c *gc.C) {
	listener := bufconn.Listen(8 * 1024)

	acceptOnceListener, closeAllowed := sshserver.NewSSHServerListener(listener)
	c.Assert(acceptOnceListener, gc.NotNil)

	done := make(chan bool, 1)

	go func() {
		defer func() {
			done <- true
		}()
		// Accept runs and sends down the channel, it is blocked then until
		// Close continues.
		acceptOnceListener.Accept()
	}()

	<-closeAllowed
	listener.Close()

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fail()
	}
}
