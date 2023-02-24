// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventqueue

import (
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/testing"
)

type eventQueueSuite struct {
	baseSuite
}

var _ = gc.Suite(&eventQueueSuite{})

func (s *eventQueueSuite) TestSubscribe(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()

	changes := make(chan changestream.ChangeEvent)
	defer close(changes)

	s.stream.EXPECT().Changes().Return(changes).AnyTimes()

	queue := New(s.stream, s.logger)
	defer workertest.DirtyKill(c, queue)

	sub, err := queue.Subscribe(changestream.Namespace("topic", changestream.Create))
	c.Assert(err, jc.ErrorIsNil)
	sub.Unsubscribe()

	workertest.CleanKill(c, queue)
}

func (s *eventQueueSuite) TestDispatch(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()

	changes := make(chan changestream.ChangeEvent)
	defer close(changes)

	s.stream.EXPECT().Changes().Return(changes).MinTimes(1)

	queue := New(s.stream, s.logger)
	defer workertest.DirtyKill(c, queue)

	sub, err := queue.Subscribe(changestream.Namespace("topic", changestream.Create))
	c.Assert(err, jc.ErrorIsNil)

	s.expectChangeEvent(changestream.Create, "topic")

	go func() {
		select {
		case changes <- s.changeEvent:
		case <-time.After(testing.ShortWait):
			c.Fatal("timed out waiting to enqueue event")
		}
	}()

	select {
	case event := <-sub.Changes():
		c.Assert(event.Type(), jc.DeepEquals, changestream.Create)
		c.Assert(event.Namespace(), jc.DeepEquals, "topic")
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for event")
	}

	sub.Unsubscribe()

	workertest.CleanKill(c, queue)
}

func (s *eventQueueSuite) TestMultipleDispatch(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()

	changes := make(chan changestream.ChangeEvent)
	defer close(changes)

	s.stream.EXPECT().Changes().Return(changes).MinTimes(1)

	queue := New(s.stream, s.logger)
	defer workertest.DirtyKill(c, queue)

	s.expectChangeEvent(changestream.Create, "topic")

	subs := make([]changestream.Subscription, 10)

	for i := 0; i < 10; i++ {
		sub, err := queue.Subscribe(changestream.Namespace("topic", changestream.Create))
		c.Assert(err, jc.ErrorIsNil)

		subs[i] = sub
	}

	go func() {
		select {
		case changes <- s.changeEvent:
		case <-time.After(testing.ShortWait):
			c.Fatal("timed out waiting to enqueue event")
		}
	}()

	for _, sub := range subs {
		select {
		case event := <-sub.Changes():
			c.Assert(event.Type(), jc.DeepEquals, changestream.Create)
			c.Assert(event.Namespace(), jc.DeepEquals, "topic")
		case <-time.After(testing.ShortWait):
			c.Fatal("timed out waiting for event")
		}
		go sub.Unsubscribe()
	}

	workertest.CleanKill(c, queue)
}

func (s *eventQueueSuite) TestSubscribeWithMultipleMasks(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()

	changes := make(chan changestream.ChangeEvent)
	defer close(changes)

	s.stream.EXPECT().Changes().Return(changes).MinTimes(1)

	queue := New(s.stream, s.logger)
	defer workertest.DirtyKill(c, queue)

	sub, err := queue.Subscribe(changestream.Namespace("topic", changestream.Create|changestream.Update))
	c.Assert(err, jc.ErrorIsNil)

	s.expectChangeEvent(changestream.Update, "topic")

	go func() {
		select {
		case changes <- s.changeEvent:
		case <-time.After(testing.ShortWait):
			c.Fatal("timed out waiting to enqueue event")
		}
	}()

	select {
	case event := <-sub.Changes():
		c.Assert(event.Type(), jc.DeepEquals, changestream.Update)
		c.Assert(event.Namespace(), jc.DeepEquals, "topic")
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for event")
	}

	sub.Unsubscribe()

	workertest.CleanKill(c, queue)
}

func (s *eventQueueSuite) TestDispatchWithMultipleMasks(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()

	changes := make(chan changestream.ChangeEvent)
	defer close(changes)

	s.stream.EXPECT().Changes().Return(changes).MinTimes(1)

	queue := New(s.stream, s.logger)
	defer workertest.DirtyKill(c, queue)

	sub, err := queue.Subscribe(changestream.Namespace("topic", changestream.Create))
	c.Assert(err, jc.ErrorIsNil)

	s.expectChangeEvent(changestream.Create|changestream.Update, "topic")

	go func() {
		select {
		case changes <- s.changeEvent:
		case <-time.After(testing.ShortWait):
			c.Fatal("timed out waiting to enqueue event")
		}
	}()

	select {
	case event := <-sub.Changes():
		c.Assert(event.Type(), jc.DeepEquals, changestream.Create|changestream.Update)
		c.Assert(event.Namespace(), jc.DeepEquals, "topic")
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for event")
	}

	sub.Unsubscribe()

	workertest.CleanKill(c, queue)
}

func (s *eventQueueSuite) TestSubscribeWithMatchingFilter(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()

	changes := make(chan changestream.ChangeEvent)
	defer close(changes)

	s.stream.EXPECT().Changes().Return(changes).MinTimes(1)

	queue := New(s.stream, s.logger)
	defer workertest.DirtyKill(c, queue)

	sub, err := queue.Subscribe(changestream.FilteredNamespace("topic", changestream.Create, func(event changestream.ChangeEvent) bool {
		return event.Namespace() == "topic"
	}))
	c.Assert(err, jc.ErrorIsNil)

	s.expectChangeEvent(changestream.Create|changestream.Update, "topic")

	go func() {
		select {
		case changes <- s.changeEvent:
		case <-time.After(testing.ShortWait):
			c.Fatal("timed out waiting to enqueue event")
		}
	}()

	select {
	case event := <-sub.Changes():
		c.Assert(event.Type(), jc.DeepEquals, changestream.Create|changestream.Update)
		c.Assert(event.Namespace(), jc.DeepEquals, "topic")
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for event")
	}

	sub.Unsubscribe()

	workertest.CleanKill(c, queue)
}

func (s *eventQueueSuite) TestSubscribeWithNonMatchingFilter(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()

	changes := make(chan changestream.ChangeEvent)
	defer close(changes)

	s.stream.EXPECT().Changes().Return(changes).MinTimes(1)

	queue := New(s.stream, s.logger)
	defer workertest.DirtyKill(c, queue)

	sub, err := queue.Subscribe(changestream.FilteredNamespace("topic", changestream.Create, func(event changestream.ChangeEvent) bool {
		return event.Namespace() != "topic"
	}))
	c.Assert(err, jc.ErrorIsNil)

	s.expectChangeEvent(changestream.Create|changestream.Update, "topic")

	go func() {
		select {
		case changes <- s.changeEvent:
		case <-time.After(testing.ShortWait):
			c.Fatal("timed out waiting to enqueue event")
		}
	}()

	select {
	case event := <-sub.Changes():
		c.Fatal("unexpected event", event)
	case <-time.After(time.Second):
	}

	sub.Unsubscribe()

	workertest.CleanKill(c, queue)
}

func (s *eventQueueSuite) TestUnsubscribeChannelClosed(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()

	changes := make(chan changestream.ChangeEvent)
	defer close(changes)

	s.stream.EXPECT().Changes().Return(changes).MinTimes(1)

	queue := New(s.stream, s.logger)
	defer workertest.DirtyKill(c, queue)

	sub, err := queue.Subscribe(changestream.FilteredNamespace("topic", changestream.Create, func(event changestream.ChangeEvent) bool {
		return event.Namespace() == "topic"
	}))
	c.Assert(err, jc.ErrorIsNil)

	s.expectChangeEvent(changestream.Create|changestream.Update, "topic")

	go func() {
		select {
		case changes <- s.changeEvent:
		case <-time.After(testing.ShortWait):
			c.Fatal("timed out waiting to enqueue event")
		}
	}()

	select {
	case event := <-sub.Changes():
		c.Assert(event.Type(), jc.DeepEquals, changestream.Create|changestream.Update)
		c.Assert(event.Namespace(), jc.DeepEquals, "topic")
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for event")
	}

	sub.Unsubscribe()

	select {
	case _, open := <-sub.Changes():
		if open {
			c.Fatal("expected subscription channel to be closed")
		}
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for event")
	}

	workertest.CleanKill(c, queue)
}
