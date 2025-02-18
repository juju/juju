package sshserver_test

import (
	"sync"
	"time"

	"github.com/juju/juju/worker/sshserver"
	"github.com/juju/testing"
	"google.golang.org/grpc/test/bufconn"
	gc "gopkg.in/check.v1"
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

	// Artificial sleep to ensure close is definitely waiting on <-closeAllowed
	time.Sleep(100 * time.Millisecond)

	go func() {
		defer wg.Done()
		// Accept runs and sends down the channel, it is blocked then until
		// Close continues.
		acceptOnceListener.Accept()
	}()

	wg.Wait()
}
