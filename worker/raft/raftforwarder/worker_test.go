// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftforwarder_test

import (
	"sync/atomic"
	"time"

	"github.com/hashicorp/raft"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/pubsub"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/raftlease"
	"github.com/juju/juju/pubsub/centralhub"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/raft/raftforwarder"
)

type workerFixture struct {
	testing.IsolationSuite
	raft     *mockRaft
	response *mockResponse
	target   *fakeTarget
	hub      *pubsub.StructuredHub
	config   raftforwarder.Config
}

func (s *workerFixture) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	err := loggo.ConfigureLoggers("TRACE")
	c.Assert(err, jc.ErrorIsNil)

	s.response = &mockResponse{}
	s.raft = &mockRaft{af: &mockApplyFuture{
		response: s.response,
	}}
	s.target = &fakeTarget{}
	s.hub = centralhub.New(names.NewMachineTag("17"))
	s.config = raftforwarder.Config{
		Hub:                  s.hub,
		Raft:                 s.raft,
		Logger:               loggo.GetLogger("raftforwarder_test"),
		PrometheusRegisterer: &noopRegisterer{},
		Topic:                "raftforwarder_test",
		Target:               s.target,
	}
}

type workerValidationSuite struct {
	workerFixture
}

var _ = gc.Suite(&workerValidationSuite{})

func (s *workerValidationSuite) TestValidateErrors(c *gc.C) {
	type test struct {
		f      func(*raftforwarder.Config)
		expect string
	}
	tests := []test{{
		func(cfg *raftforwarder.Config) { cfg.Raft = nil },
		"nil Raft not valid",
	}, {
		func(cfg *raftforwarder.Config) { cfg.Hub = nil },
		"nil Hub not valid",
	}, {
		func(cfg *raftforwarder.Config) { cfg.Logger = nil },
		"nil Logger not valid",
	}, {
		func(cfg *raftforwarder.Config) { cfg.PrometheusRegisterer = nil },
		"nil PrometheusRegisterer not valid",
	}, {
		func(cfg *raftforwarder.Config) { cfg.Topic = "" },
		"empty Topic not valid",
	}, {
		func(cfg *raftforwarder.Config) { cfg.Target = nil },
		"nil Target not valid",
	}}
	for i, test := range tests {
		c.Logf("test #%d (%s)", i, test.expect)
		s.testValidateError(c, test.f, test.expect)
	}
}

func (s *workerValidationSuite) testValidateError(c *gc.C, f func(*raftforwarder.Config), expect string) {
	config := s.config
	f(&config)
	w, err := raftforwarder.NewWorker(config)
	if !c.Check(err, gc.NotNil) {
		workertest.DirtyKill(c, w)
		return
	}
	c.Check(w, gc.IsNil)
	c.Check(err, gc.ErrorMatches, expect)
}

type workerSuite struct {
	workerFixture
	worker worker.Worker
	resps  chan raftlease.ForwardResponse
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) SetUpTest(c *gc.C) {
	s.workerFixture.SetUpTest(c)
	s.resps = make(chan raftlease.ForwardResponse)

	// Use a local variable to send to the channel in the callback, so
	// we don't get races when a subsequent test overwrites s.resps
	// with a new channel.
	resps := s.resps
	unsubscribe, err := s.hub.Subscribe(
		"response",
		func(_ string, resp raftlease.ForwardResponse, err error) {
			c.Check(err, jc.ErrorIsNil)
			resps <- resp
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) { unsubscribe() })

	worker, err := raftforwarder.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) {
		workertest.DirtyKill(c, worker)
	})
	s.worker = worker
}

func (s *workerSuite) TestCleanKill(c *gc.C) {
	workertest.CleanKill(c, s.worker)
}

func (s *workerSuite) TestSuccess(c *gc.C) {
	_, err := s.hub.Publish("raftforwarder_test", raftlease.ForwardRequest{
		Command:       "myanmar",
		ResponseTopic: "response",
	})
	c.Assert(err, jc.ErrorIsNil)

	select {
	case resp := <-s.resps:
		c.Assert(resp, gc.DeepEquals, raftlease.ForwardResponse{})
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for response")
	}

	s.raft.CheckCall(c, 0, "Apply", []byte("myanmar"), 5*time.Second)
	s.response.CheckCall(c, 0, "Notify", s.target)
}

func (s *workerSuite) TestApplyError(c *gc.C) {
	s.raft.af.SetErrors(errors.Errorf("boom"))
	_, err := s.hub.Publish("raftforwarder_test", raftlease.ForwardRequest{
		Command:       "france",
		ResponseTopic: "response",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = workertest.CheckKilled(c, s.worker)
	c.Assert(err, gc.ErrorMatches, "applying command: boom")

	select {
	case <-s.resps:
		c.Fatalf("unexpected response")
	case <-time.After(coretesting.ShortWait):
	}
}

func (s *workerSuite) TestBadResponseType(c *gc.C) {
	s.raft.af.response = "23 skidoo!"
	_, err := s.hub.Publish("raftforwarder_test", raftlease.ForwardRequest{
		Command:       "france",
		ResponseTopic: "response",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = workertest.CheckKilled(c, s.worker)
	c.Assert(err, gc.ErrorMatches, `applying command: expected an FSMResponse, got string: "23 skidoo!"`)

	select {
	case <-s.resps:
		c.Fatalf("unexpected response")
	case <-time.After(coretesting.ShortWait):
	}
}

func (s *workerSuite) TestResponseGenericError(c *gc.C) {
	s.response.SetErrors(errors.Errorf("help!"))
	_, err := s.hub.Publish("raftforwarder_test", raftlease.ForwardRequest{
		Command:       "france",
		ResponseTopic: "response",
	})
	c.Assert(err, jc.ErrorIsNil)

	select {
	case resp := <-s.resps:
		c.Assert(resp, gc.DeepEquals, raftlease.ForwardResponse{
			Error: &raftlease.ResponseError{"help!", "error"},
		})
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for response")
	}
}

func (s *workerSuite) TestResponseSingletonError(c *gc.C) {
	s.response.SetErrors(errors.Annotate(lease.ErrInvalid, "some context"))
	_, err := s.hub.Publish("raftforwarder_test", raftlease.ForwardRequest{
		Command:       "france",
		ResponseTopic: "response",
	})
	c.Assert(err, jc.ErrorIsNil)

	select {
	case resp := <-s.resps:
		c.Assert(resp, gc.DeepEquals, raftlease.ForwardResponse{
			Error: &raftlease.ResponseError{"some context: invalid lease operation", "invalid"},
		})
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for response")
	}
}

func (s *workerSuite) TestHandlesRequestsConcurrently(c *gc.C) {
	resps2 := make(chan raftlease.ForwardResponse)
	unsubscribe, err := s.hub.Subscribe(
		"response2",
		func(_ string, resp raftlease.ForwardResponse, err error) {
			c.Check(err, jc.ErrorIsNil)
			resps2 <- resp
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	defer unsubscribe()

	var calls int32
	started := make(chan struct{})
	finish := make(chan struct{})
	s.raft.af.callback = func() {
		call := atomic.AddInt32(&calls, 1)
		// The first call blocks until we signal it.
		if call == 1 {
			close(started)
			<-finish
		}
	}

	// Send a request (response to come on s.resps) that blocks.
	_, err = s.hub.Publish("raftforwarder_test", raftlease.ForwardRequest{
		Command:       "myanmar",
		ResponseTopic: "response",
	})
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-started:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for first request to start")
	}

	// Send a request (response to come on s.resps) that blocks.
	_, err = s.hub.Publish("raftforwarder_test", raftlease.ForwardRequest{
		Command:       "myanmar",
		ResponseTopic: "response2",
	})
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-resps2:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for response from second request")
	}

	select {
	case <-s.resps:
		c.Fatalf("got response from first request too early")
	case <-time.After(coretesting.ShortWait):
	}

	close(finish)

	select {
	case <-s.resps:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for response from first request")
	}
}

type mockRaft struct {
	testing.Stub
	af *mockApplyFuture
}

func (r *mockRaft) Apply(cmd []byte, timeout time.Duration) raft.ApplyFuture {
	r.AddCall("Apply", cmd, timeout)
	return r.af
}

type mockApplyFuture struct {
	raft.IndexFuture
	testing.Stub
	response interface{}
	callback func()
}

func (f *mockApplyFuture) Error() error {
	if f.callback != nil {
		f.callback()
	}
	f.AddCall("Error")
	return f.NextErr()
}

func (f *mockApplyFuture) Response() interface{} {
	f.AddCall("Response")
	return f.response
}

type mockResponse struct {
	testing.Stub
}

func (r *mockResponse) Error() error {
	return r.NextErr()
}

func (r *mockResponse) Notify(target raftlease.NotifyTarget) {
	r.AddCall("Notify", target)
}

type fakeTarget struct {
	raftlease.NotifyTarget
}
