// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftnotifier_test

import (
	"github.com/juju/clock"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"
	"github.com/pkg/errors"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/raft/notifyproxy"
	"github.com/juju/juju/core/raftlease"
	"github.com/juju/juju/worker/raft/raftnotifier"
)

type workerFixture struct {
	testing.IsolationSuite

	notifyProxy *notifyproxy.NotifyProxy
	target      *fakeTarget
	config      raftnotifier.Config
}

func (s *workerFixture) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	err := loggo.ConfigureLoggers("TRACE")
	c.Assert(err, jc.ErrorIsNil)

	s.target = &fakeTarget{}
	s.notifyProxy = notifyproxy.NewBlocking(clock.WallClock)

	s.config = raftnotifier.Config{
		Logger:       loggo.GetLogger("raftnotifier_test"),
		NotifyTarget: s.target,
		NotifyProxy:  s.notifyProxy,
	}

	s.AddCleanup(func(c *gc.C) {
		s.notifyProxy.Close()
	})
}

type workerValidationSuite struct {
	workerFixture
}

var _ = gc.Suite(&workerValidationSuite{})

func (s *workerValidationSuite) TestValidateErrors(c *gc.C) {
	type test struct {
		f      func(*raftnotifier.Config)
		expect string
	}
	tests := []test{{
		func(cfg *raftnotifier.Config) { cfg.Logger = nil },
		"nil Logger not valid",
	}, {
		func(cfg *raftnotifier.Config) { cfg.NotifyProxy = nil },
		"nil NotifyProxy not valid",
	}, {
		func(cfg *raftnotifier.Config) { cfg.NotifyTarget = nil },
		"nil NotifyTarget not valid",
	}}
	for i, test := range tests {
		c.Logf("test #%d (%s)", i, test.expect)
		s.testValidateError(c, test.f, test.expect)
	}
}

func (s *workerValidationSuite) testValidateError(c *gc.C, f func(*raftnotifier.Config), expect string) {
	config := s.config
	f(&config)
	w, err := raftnotifier.NewWorker(config)
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
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) SetUpTest(c *gc.C) {
	s.workerFixture.SetUpTest(c)

	worker, err := raftnotifier.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) {
		workertest.DirtyKill(c, worker)
	})
	s.worker = worker
}

func (s *workerSuite) TestCleanKill(c *gc.C) {
	workertest.CleanKill(c, s.worker)
}

func (s *workerSuite) TestClaimedSuccess(c *gc.C) {
	key := lease.Key{Namespace: "ns", ModelUUID: "model", Lease: "lease"}
	err := s.notifyProxy.Claimed(key, "meshuggah")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.target.claims, gc.DeepEquals, []Claimed{{
		Key:    key,
		Holder: "meshuggah",
	}})
}

func (s *workerSuite) TestClaimedError(c *gc.C) {
	s.target.err = errors.Errorf("boom")

	key := lease.Key{Namespace: "ns", ModelUUID: "model", Lease: "lease"}
	err := s.notifyProxy.Claimed(key, "meshuggah")
	c.Assert(err, gc.ErrorMatches, "boom")

	err = workertest.CheckKilled(c, s.worker)
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *workerSuite) TestExpiriesSuccess(c *gc.C) {
	key := lease.Key{Namespace: "ns", ModelUUID: "model", Lease: "lease"}
	err := s.notifyProxy.Expirations([]raftlease.Expired{{
		Key:    key,
		Holder: "meshuggah",
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.target.expirations, gc.DeepEquals, []Expirations{{
		Expirations: []raftlease.Expired{{
			Key:    key,
			Holder: "meshuggah",
		}},
	}})
}

func (s *workerSuite) TestExpiriesError(c *gc.C) {
	s.target.err = errors.Errorf("boom")

	key := lease.Key{Namespace: "ns", ModelUUID: "model", Lease: "lease"}
	err := s.notifyProxy.Expirations([]raftlease.Expired{{
		Key:    key,
		Holder: "meshuggah",
	}})
	c.Assert(err, gc.ErrorMatches, "boom")

	err = workertest.CheckKilled(c, s.worker)
	c.Assert(err, gc.ErrorMatches, "boom")
}

type Claimed struct {
	Key    lease.Key
	Holder string
}

type Expirations struct {
	Expirations []raftlease.Expired
}

type fakeTarget struct {
	claims      []Claimed
	expirations []Expirations
	err         error
}

// Claimed will be called when a new lease has been claimed.
func (t *fakeTarget) Claimed(key lease.Key, holder string) error {
	t.claims = append(t.claims, Claimed{
		Key:    key,
		Holder: holder,
	})
	return t.err
}

// Expirations will be called when a set of existing leases have expired.
func (t *fakeTarget) Expirations(expires []raftlease.Expired) error {
	t.expirations = append(t.expirations, Expirations{
		Expirations: expires,
	})
	return t.err
}
