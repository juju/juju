// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftlease_test

import (
	"fmt"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/pubsub"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/core/globalclock"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/raftlease"
	coretesting "github.com/juju/juju/testing"
)

type storeSuite struct {
	testing.IsolationSuite

	clock *testclock.Clock
	fsm   *fakeFSM
	hub   *pubsub.StructuredHub
	store *raftlease.Store
}

var _ = gc.Suite(&storeSuite{})

func (s *storeSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	startTime, err := time.Parse(time.RFC3339, "2018-08-08T08:08:08+08:00")
	c.Assert(err, jc.ErrorIsNil)
	s.clock = testclock.NewClock(startTime)
	s.fsm = &fakeFSM{
		leases:     make(map[lease.Key]lease.Info),
		globalTime: s.clock.Now(),
	}
	s.hub = pubsub.NewStructuredHub(nil)
	s.store = raftlease.NewStore(raftlease.StoreConfig{
		FSM:          s.fsm,
		Hub:          s.hub,
		Trapdoor:     FakeTrapdoor,
		RequestTopic: "lease.request",
		ResponseTopic: func(reqID uint64) string {
			return fmt.Sprintf("lease.request.%d", reqID)
		},
		Clock:          s.clock,
		ForwardTimeout: time.Second,
	})
}

func (s *storeSuite) TestClaim(c *gc.C) {
	s.handleHubRequest(c,
		func() {
			err := s.store.ClaimLease(
				lease.Key{"warframe", "rhino", "prime"},
				lease.Request{"lotus", time.Second},
			)
			c.Assert(err, jc.ErrorIsNil)
		},

		raftlease.Command{
			Version:   1,
			Operation: raftlease.OperationClaim,
			Namespace: "warframe",
			ModelUUID: "rhino",
			Lease:     "prime",
			Holder:    "lotus",
			Duration:  time.Second,
		},
		func(req raftlease.ForwardRequest) {
			_, err := s.hub.Publish(
				req.ResponseTopic,
				raftlease.ForwardResponse{},
			)
			c.Check(err, jc.ErrorIsNil)
		},
	)
}

func (s *storeSuite) TestClaimTimeout(c *gc.C) {
	s.handleHubRequest(c,
		func() {
			errChan := make(chan error)
			go func() {
				errChan <- s.store.ClaimLease(
					lease.Key{"warframe", "vauban", "prime"},
					lease.Request{"vor", time.Second},
				)
			}()
			// Jump time forward further than the 1-second forward
			// timeout.
			s.clock.WaitAdvance(2*time.Second, coretesting.LongWait, 1)
			select {
			case err := <-errChan:
				c.Assert(err, jc.Satisfies, lease.IsTimeout)
			case <-time.After(coretesting.LongWait):
				c.Fatalf("timed out waiting for claim error")
			}
		},

		raftlease.Command{
			Version:   1,
			Operation: raftlease.OperationClaim,
			Namespace: "warframe",
			ModelUUID: "vauban",
			Lease:     "prime",
			Holder:    "vor",
			Duration:  time.Second,
		},
		func(req raftlease.ForwardRequest) {
			// We never send a response, to trigger a timeout.
		},
	)
}

func (s *storeSuite) TestClaimInvalid(c *gc.C) {
	s.handleHubRequest(c,
		func() {
			err := s.store.ClaimLease(
				lease.Key{"warframe", "volt", "umbra"},
				lease.Request{"maroo", 3 * time.Second},
			)
			c.Assert(err, jc.Satisfies, lease.IsInvalid)
		},

		raftlease.Command{
			Version:   1,
			Operation: raftlease.OperationClaim,
			Namespace: "warframe",
			ModelUUID: "volt",
			Lease:     "umbra",
			Holder:    "maroo",
			Duration:  3 * time.Second,
		},
		func(req raftlease.ForwardRequest) {
			_, err := s.hub.Publish(
				req.ResponseTopic,
				raftlease.ForwardResponse{
					Error: &raftlease.ResponseError{
						Code: "invalid",
					},
				},
			)
			c.Check(err, jc.ErrorIsNil)
		},
	)
}

func (s *storeSuite) TestExtend(c *gc.C) {
	s.handleHubRequest(c,
		func() {
			err := s.store.ExtendLease(
				lease.Key{"warframe", "frost", "prime"},
				lease.Request{"konzu", time.Second},
			)
			c.Assert(err, jc.ErrorIsNil)
		},

		raftlease.Command{
			Version:   1,
			Operation: raftlease.OperationExtend,
			Namespace: "warframe",
			ModelUUID: "frost",
			Lease:     "prime",
			Holder:    "konzu",
			Duration:  time.Second,
		},
		func(req raftlease.ForwardRequest) {
			_, err := s.hub.Publish(
				req.ResponseTopic,
				raftlease.ForwardResponse{},
			)
			c.Check(err, jc.ErrorIsNil)
		},
	)
}

func (s *storeSuite) TestExpire(c *gc.C) {
	err := s.store.ExpireLease(
		lease.Key{"warframe", "oberon", "prime"},
	)
	c.Assert(err, jc.Satisfies, lease.IsInvalid)
}

func (s *storeSuite) TestLeases(c *gc.C) {
	in5Seconds := s.clock.Now().Add(5 * time.Second)
	in10Seconds := s.clock.Now().Add(10 * time.Second)
	lease1 := lease.Key{"quam", "olim", "abrahe"}
	lease2 := lease.Key{"la", "cry", "mosa"}
	s.fsm.leases[lease1] = lease.Info{
		Holder: "verdi",
		Expiry: in10Seconds,
	}
	s.fsm.leases[lease2] = lease.Info{
		Holder: "mozart",
		Expiry: in5Seconds,
	}
	result := s.store.Leases()
	c.Assert(len(result), gc.Equals, 2)

	r1 := result[lease1]
	c.Assert(r1.Holder, gc.Equals, "verdi")
	c.Assert(r1.Expiry, gc.Equals, in10Seconds)

	// Can't compare trapdoors directly.
	var out string
	err := r1.Trapdoor(&out)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, gc.Equals, "{quam olim abrahe} held by verdi")

	r2 := result[lease2]
	c.Assert(r2.Holder, gc.Equals, "mozart")
	c.Assert(r2.Expiry, gc.Equals, in5Seconds)

	err = r2.Trapdoor(&out)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, gc.Equals, "{la cry mosa} held by mozart")
}

func (s *storeSuite) TestPin(c *gc.C) {
	machineTag := names.NewMachineTag("0")
	s.handleHubRequest(c,
		func() {
			err := s.store.PinLease(
				lease.Key{"warframe", "frost", "prime"},
				machineTag,
			)
			c.Assert(err, jc.ErrorIsNil)
		},
		raftlease.Command{
			Version:   1,
			Operation: raftlease.OperationPin,
			Namespace: "warframe",
			ModelUUID: "frost",
			Lease:     "prime",
			PinEntity: machineTag.String(),
		},
		func(req raftlease.ForwardRequest) {
			_, err := s.hub.Publish(
				req.ResponseTopic,
				raftlease.ForwardResponse{},
			)
			c.Check(err, jc.ErrorIsNil)
		},
	)
}

func (s *storeSuite) TestUnpin(c *gc.C) {
	machineTag := names.NewMachineTag("0")
	s.handleHubRequest(c,
		func() {
			err := s.store.UnpinLease(
				lease.Key{"warframe", "frost", "prime"},
				machineTag,
			)
			c.Assert(err, jc.ErrorIsNil)
		},
		raftlease.Command{
			Version:   1,
			Operation: raftlease.OperationUnpin,
			Namespace: "warframe",
			ModelUUID: "frost",
			Lease:     "prime",
			PinEntity: machineTag.String(),
		},
		func(req raftlease.ForwardRequest) {
			_, err := s.hub.Publish(
				req.ResponseTopic,
				raftlease.ForwardResponse{},
			)
			c.Check(err, jc.ErrorIsNil)
		},
	)
}

// handleHubRequest takes the action that triggers the request, the
// expected command, and a function that will be run to make checks on
// the request and send the response back.
func (s *storeSuite) handleHubRequest(
	c *gc.C,
	action func(),
	expectCommand raftlease.Command,
	responder func(raftlease.ForwardRequest),
) {
	expectedBytes := marshal(c, expectCommand)
	called := make(chan struct{})
	unsubscribe, err := s.hub.Subscribe(
		"lease.request",
		func(_ string, req raftlease.ForwardRequest, err error) {
			defer close(called)
			c.Check(err, jc.ErrorIsNil)
			c.Check(req.Command, gc.DeepEquals, expectedBytes)
			responder(req)
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	defer unsubscribe()

	action()
	select {
	case <-called:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for hub message")
	}
}

func (s *storeSuite) TestAdvance(c *gc.C) {
	fromTime := s.clock.Now()

	s.handleHubRequest(c,
		func() {
			err := s.store.Advance(10 * time.Second)
			c.Assert(err, jc.ErrorIsNil)
		},
		raftlease.Command{
			Version:   1,
			Operation: raftlease.OperationSetTime,
			OldTime:   fromTime,
			NewTime:   fromTime.Add(10 * time.Second),
		},
		func(req raftlease.ForwardRequest) {
			c.Check(req.ResponseTopic, gc.Equals, "lease.request.1")
			_, err := s.hub.Publish(
				req.ResponseTopic,
				raftlease.ForwardResponse{},
			)
			c.Check(err, jc.ErrorIsNil)
		},
	)
	// The store time advances, as seen in the next update.
	s.handleHubRequest(c,
		func() {
			err := s.store.Advance(5 * time.Second)
			c.Assert(err, jc.ErrorIsNil)
		},
		raftlease.Command{
			Version:   1,
			Operation: raftlease.OperationSetTime,
			OldTime:   fromTime.Add(10 * time.Second),
			NewTime:   fromTime.Add(15 * time.Second),
		},
		func(req raftlease.ForwardRequest) {
			c.Check(req.ResponseTopic, gc.Equals, "lease.request.2")
			_, err := s.hub.Publish(
				req.ResponseTopic,
				raftlease.ForwardResponse{},
			)
			c.Check(err, jc.ErrorIsNil)
		},
	)
}

func (s *storeSuite) TestAdvanceConcurrentUpdate(c *gc.C) {
	fromTime := s.clock.Now()
	plus5Sec := fromTime.Add(5 * time.Second)
	plus10Sec := fromTime.Add(10 * time.Second)
	s.fsm.globalTime = plus5Sec

	s.handleHubRequest(c,
		func() {
			err := s.store.Advance(10 * time.Second)
			c.Assert(err, jc.Satisfies, globalclock.IsConcurrentUpdate)
		},
		raftlease.Command{
			Version:   1,
			Operation: raftlease.OperationSetTime,
			OldTime:   fromTime,
			NewTime:   plus10Sec,
		},
		func(req raftlease.ForwardRequest) {
			_, err := s.hub.Publish(
				req.ResponseTopic,
				raftlease.ForwardResponse{
					Error: &raftlease.ResponseError{
						Code: "concurrent-update",
					},
				},
			)
			c.Check(err, jc.ErrorIsNil)
		},
	)

	// Check that the store updates time from the FSM for when we try
	// again.
	s.handleHubRequest(c,
		func() {
			err := s.store.Advance(10 * time.Second)
			c.Assert(err, jc.ErrorIsNil)
		},
		raftlease.Command{
			Version:   1,
			Operation: raftlease.OperationSetTime,
			OldTime:   plus5Sec,
			NewTime:   fromTime.Add(15 * time.Second),
		},
		func(req raftlease.ForwardRequest) {
			c.Check(req.ResponseTopic, gc.Equals, "lease.request.2")
			_, err := s.hub.Publish(
				req.ResponseTopic,
				raftlease.ForwardResponse{},
			)
			c.Check(err, jc.ErrorIsNil)
		},
	)
}

func (s *storeSuite) TestAsResponseError(c *gc.C) {
	c.Assert(
		raftlease.AsResponseError(lease.ErrInvalid),
		gc.DeepEquals,
		&raftlease.ResponseError{
			"invalid lease operation",
			"invalid",
		},
	)
	c.Assert(
		raftlease.AsResponseError(globalclock.ErrConcurrentUpdate),
		gc.DeepEquals,
		&raftlease.ResponseError{
			"clock was updated concurrently, retry",
			"concurrent-update",
		},
	)
	c.Assert(
		raftlease.AsResponseError(errors.Errorf("generic")),
		gc.DeepEquals,
		&raftlease.ResponseError{
			"generic",
			"error",
		},
	)
}

func (s *storeSuite) TestRecoverError(c *gc.C) {
	c.Assert(raftlease.RecoverError(nil), gc.Equals, nil)
	re := func(msg, code string) error {
		return raftlease.RecoverError(&raftlease.ResponseError{
			Message: msg,
			Code:    code,
		})
	}
	c.Assert(re("", "invalid"), jc.Satisfies, lease.IsInvalid)
	c.Assert(re("", "concurrent-update"), jc.Satisfies, globalclock.IsConcurrentUpdate)
	c.Assert(re("something", "else"), gc.ErrorMatches, "something")
}

type fakeFSM struct {
	testing.Stub
	leases     map[lease.Key]lease.Info
	globalTime time.Time
}

func (f *fakeFSM) Leases(t time.Time) map[lease.Key]lease.Info {
	f.AddCall("Leases", t)
	return f.leases
}

func (f *fakeFSM) GlobalTime() time.Time {
	return f.globalTime
}

func FakeTrapdoor(key lease.Key, holder string) lease.Trapdoor {
	return func(out interface{}) error {
		if s, ok := out.(*string); ok {
			*s = fmt.Sprintf("%v held by %s", key, holder)
			return nil
		}
		return errors.Errorf("bad input")
	}
}

func marshal(c *gc.C, command raftlease.Command) []byte {
	result, err := command.Marshal()
	c.Assert(err, jc.ErrorIsNil)
	return result
}
