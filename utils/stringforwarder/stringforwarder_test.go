// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stringforwarder_test

import (
	"sync"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/utils/stringforwarder"
)

type StringForwarderSuite struct{}

var _ = gc.Suite(&StringForwarderSuite{})

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

func (*StringForwarderSuite) TestReceives(c *gc.C) {
	var messages []string
	received := make(chan struct{}, 10)
	forwarder := stringforwarder.New(func(msg string) {
		messages = append(messages, msg)
		received <- struct{}{}
	})
	forwarder.Forward("one")
	waitFor(c, received)
	c.Check(forwarder.Stop(), gc.Equals, uint64(0))
	c.Check(messages, gc.DeepEquals, []string{"one"})
}

func noopCallback(string) {
}

func (*StringForwarderSuite) TestStopIsReentrant(c *gc.C) {
	forwarder := stringforwarder.New(noopCallback)
	forwarder.Stop()
	forwarder.Stop()
}

func (*StringForwarderSuite) TestMessagesDroppedAfterStop(c *gc.C) {
	var messages []string
	forwarder := stringforwarder.New(func(msg string) {
		messages = append(messages, msg)
	})
	forwarder.Stop()
	forwarder.Forward("one")
	forwarder.Forward("two")
	forwarder.Stop()
	c.Check(messages, gc.HasLen, 0)
}

func (*StringForwarderSuite) TestAllDroppedWithNoCallback(c *gc.C) {
	forwarder := stringforwarder.New(nil)
	forwarder.Forward("one")
	forwarder.Forward("two")
	forwarder.Forward("three")
	c.Check(forwarder.Stop(), gc.Equals, uint64(3))
}

func (*StringForwarderSuite) TestMessagesDroppedWhenBusy(c *gc.C) {
	var messages []string
	received := make(chan struct{}, 10)
	next := make(chan struct{})
	blockingCallback := func(msg string) {
		waitFor(c, next)
		messages = append(messages, msg)
		sendEvent(c, received)
	}
	forwarder := stringforwarder.New(blockingCallback)
	forwarder.Forward("first")
	forwarder.Forward("second")
	forwarder.Forward("third")
	// At this point we should have started processing "first", but the
	// other two messages are dropped.
	sendEvent(c, next)
	waitFor(c, received)
	// now we should be ready to get another message
	forwarder.Forward("fourth")
	forwarder.Forward("fifth")
	// finish fourth
	sendEvent(c, next)
	waitFor(c, received)
	dropCount := forwarder.Stop()
	c.Check(messages, gc.DeepEquals, []string{"first", "fourth"})
	c.Check(dropCount, gc.Equals, uint64(3))
}

func (*StringForwarderSuite) TestRace(c *gc.C) {
	forwarder := stringforwarder.New(noopCallback)
	stop := make(chan struct{})
	wg := &sync.WaitGroup{}
	f := func(wg *sync.WaitGroup) {
		wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				forwarder.Forward("next message")
			}
		}
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go f(wg)
	}
	wg.Wait()
	time.Sleep(10 * time.Millisecond)
	close(stop)
	count := forwarder.Stop()
	c.Check(count, jc.GreaterThan, uint64(0))
}
