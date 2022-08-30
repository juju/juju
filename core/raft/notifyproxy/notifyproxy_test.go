// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package notifyproxy

import (
	"fmt"
	"time"

	"github.com/juju/clock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/raftlease"
)

type NotifyProxySuite struct {
	testing.IsolationSuite

	proxy *NotifyProxy
}

var _ = gc.Suite(&NotifyProxySuite{})

func (s *NotifyProxySuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.proxy = NewNonBlocking(clock.WallClock)
	s.AddCleanup(func(*gc.C) { _ = s.proxy.Close() })
}

func (s *NotifyProxySuite) TestSendingWithNoWaiting(c *gc.C) {
	key := lease.Key{Namespace: "ns", ModelUUID: "model", Lease: "lease"}
	err := s.proxy.Claimed(key, "meshuggah")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *NotifyProxySuite) TestSendingWithNoWaitingOverflowsBuffer(c *gc.C) {
	// Issue the claimed commands,
	for i := 0; i < BufferSize+2; i++ {
		key := lease.Key{Namespace: "ns", ModelUUID: "model", Lease: "lease"}
		err := s.proxy.Claimed(key, fmt.Sprintf("meshuggah%d", i))
		c.Assert(err, jc.ErrorIsNil)
	}

	// Once all claimed commands have been issued, then start to consume them.
	done := make(chan struct{})
	results := make([]Notification, 0)
	go func() {
		defer close(done)

		for notes := range s.proxy.Notifications() {
			for _, note := range notes {
				note.ErrorResponse(nil)
				results = append(results, note)
			}
			if len(results) >= BufferSize {
				return
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting")
	}

	// As we overflowed the buffer, we should only get the buffered amount, not
	// all of them.
	for k, note := range results {
		c.Assert(note.Type(), gc.Equals, Claimed)
		expiry, ok := note.(ClaimedNote)
		c.Assert(ok, jc.IsTrue)
		c.Assert(expiry.Holder, gc.Equals, fmt.Sprintf("meshuggah%d", k+2))
	}
}

func (s *NotifyProxySuite) TestClaimed(c *gc.C) {
	done := make(chan struct{})
	results := make([]Notification, 0)
	go func() {
		for notes := range s.proxy.Notifications() {
			for _, note := range notes {
				note.ErrorResponse(nil)
				results = append(results, note)
			}
		}
		close(done)
	}()

	key := lease.Key{Namespace: "ns", ModelUUID: "model", Lease: "lease"}
	err := s.proxy.Claimed(key, "meshuggah")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.proxy.Close(), jc.ErrorIsNil)
	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting")
	}

	c.Assert(results, gc.HasLen, 1)

	for _, note := range results {
		c.Assert(note.Type(), gc.Equals, Claimed)
		expiry, ok := note.(ClaimedNote)
		c.Assert(ok, jc.IsTrue)
		c.Assert(expiry.Holder, gc.Equals, "meshuggah")
	}
}

func (s *NotifyProxySuite) TestExpirations(c *gc.C) {
	done := make(chan struct{})
	results := make([]Notification, 0)
	go func() {
		for notes := range s.proxy.Notifications() {
			for _, note := range notes {
				note.ErrorResponse(nil)
				results = append(results, note)
			}
		}
		close(done)
	}()

	expected := []raftlease.Expired{{
		Key:    lease.Key{Namespace: "ns", ModelUUID: "model", Lease: "lease"},
		Holder: "meshuggah",
	}}
	err := s.proxy.Expirations(expected)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.proxy.Close(), jc.ErrorIsNil)
	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting")
	}

	c.Assert(results, gc.HasLen, 1)

	for _, note := range results {
		c.Assert(note.Type(), gc.Equals, Expirations)
		expiry, ok := note.(ExpirationsNote)
		c.Assert(ok, jc.IsTrue)
		c.Assert(expiry.Expirations, jc.DeepEquals, expected)
	}
}

func (s *NotifyProxySuite) TestExpirationsWithBatch(c *gc.C) {
	done := make(chan struct{})
	results := make([]Notification, 0)
	go func() {
		for notes := range s.proxy.Notifications() {
			for _, note := range notes {
				note.ErrorResponse(nil)
				results = append(results, note)
			}
		}
		close(done)
	}()

	expected := []raftlease.Expired{{
		Key:    lease.Key{Namespace: "ns", ModelUUID: "model1", Lease: "lease1"},
		Holder: "meshuggah",
	}, {
		Key:    lease.Key{Namespace: "ns", ModelUUID: "model2", Lease: "lease2"},
		Holder: "nadir",
	}}
	err := s.proxy.Expirations(expected)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.proxy.Close(), jc.ErrorIsNil)
	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting")
	}

	c.Assert(results, gc.HasLen, 1)

	for _, note := range results {
		c.Assert(note.Type(), gc.Equals, Expirations)
		expiry, ok := note.(ExpirationsNote)
		c.Assert(ok, jc.IsTrue)
		c.Assert(expiry.Expirations, jc.DeepEquals, expected)
	}
}
