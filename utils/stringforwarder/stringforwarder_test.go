// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stringforwarder_test

import (
	"time"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/utils/stringforwarder"

	gc "gopkg.in/check.v1"
)

type stringForwarderSuite struct{}

var _ = gc.Suite(&stringForwarderSuite{})

func (*stringForwarderSuite) TestReceives(c *gc.C) {
	messages := make([]string, 0)
	received := make(chan struct{}, 10)
	forwarder := stringforwarder.NewStringForwarder(func(msg string) {
		messages = append(messages, msg)
		received <- struct{}{}
	})
	forwarder.Receive("one")
	select {
	case <-received:
	case <-time.After(coretesting.LongWait):
		c.Errorf("timeout waiting for a message to be received")
	}
	c.Check(forwarder.Stop(), gc.Equals, 0)
	c.Check(messages, gc.DeepEquals, []string{"one"})
}

func noopCallback(string) {
}

func (*stringForwarderSuite) TestStopIsReentrant(c *gc.C) {
	forwarder := stringforwarder.NewStringForwarder(noopCallback)
	forwarder.Stop()
	forwarder.Stop()
}

func (*stringForwarderSuite) TestMessagesDroppedAfterStop(c *gc.C) {
	messages := make([]string, 0)
	forwarder := stringforwarder.NewStringForwarder(func(msg string) {
		messages = append(messages, msg)
	})
	forwarder.Stop()
	forwarder.Receive("one")
	forwarder.Receive("two")
	forwarder.Stop()
	c.Check(messages, gc.DeepEquals, []string{})
}

func (*stringForwarderSuite) TestMessagesDroppedWhenBusy(c *gc.C) {
	messages := make([]string, 0)
	next := make(chan struct{})
	blockingCallback := func(msg string) {
		select {
		case <-next:
			messages = append(messages, msg)
		case <-time.After(coretesting.LongWait):
			c.Error("timeout waiting for next")
			return
		}
	}
	forwarder := stringforwarder.NewStringForwarder(blockingCallback)
	forwarder.Receive("first")
	forwarder.Receive("second")
	forwarder.Receive("third")
	// At this point we should have started processing "first", but the
	// other two messages are dropped.
	select {
	case next <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Error("failed to send next signal")
	}
	// now we should be ready to get another message
	forwarder.Receive("fourth")
	forwarder.Receive("fifth")
	// finish fourth
	select {
	case next <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Error("failed to send next signal")
	}
	dropCount := forwarder.Stop()
	c.Check(messages, gc.DeepEquals, []string{"first", "fourth"})
	c.Check(dropCount, gc.Equals, 3)
}
