// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stringforwarder_test

import (
	"time"

	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/utils/stringforwarder"
)

type stringForwarderSuite struct{}

var _ = gc.Suite(&stringForwarderSuite{})

// waitFor event to happen, or timeout and fail the test
func waitFor(c *gc.C, event <-chan struct{}) {
	select {
	case <-event:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timeout waiting for event")
	}
}

// sendEvent will send a message on a channel, or timeout if the channel is
// never available and fail the test.
func sendEvent(c *gc.C, event chan struct{}) {
	select {
	case event <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("failed to send the event")
	}
}

func (*stringForwarderSuite) TestReceives(c *gc.C) {
	messages := make([]string, 0)
	received := make(chan struct{}, 10)
	forwarder := stringforwarder.NewStringForwarder(func(msg string) {
		messages = append(messages, msg)
		received <- struct{}{}
	})
	forwarder.Receive("one")
	waitFor(c, received)
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

func (*stringForwarderSuite) TestAllDroppedWithNoCallback(c *gc.C) {
	forwarder := stringforwarder.NewStringForwarder(nil)
	forwarder.Receive("one")
	forwarder.Receive("two")
	forwarder.Receive("three")
	c.Check(forwarder.Stop(), gc.Equals, 3)
}

func (*stringForwarderSuite) TestMessagesDroppedWhenBusy(c *gc.C) {
	messages := make([]string, 0)
	received := make(chan struct{}, 10)
	next := make(chan struct{})
	blockingCallback := func(msg string) {
		waitFor(c, next)
		messages = append(messages, msg)
		sendEvent(c, received)
	}
	forwarder := stringforwarder.NewStringForwarder(blockingCallback)
	forwarder.Receive("first")
	forwarder.Receive("second")
	forwarder.Receive("third")
	// At this point we should have started processing "first", but the
	// other two messages are dropped.
	sendEvent(c, next)
	waitFor(c, received)
	// now we should be ready to get another message
	forwarder.Receive("fourth")
	forwarder.Receive("fifth")
	// finish fourth
	sendEvent(c, next)
	waitFor(c, received)
	dropCount := forwarder.Stop()
	c.Check(messages, gc.DeepEquals, []string{"first", "fourth"})
	c.Check(dropCount, gc.Equals, 3)
}
