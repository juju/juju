// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package singular_test

import (
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/lease"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/singular"
)

type fixture struct {
	testing.Stub
}

func newFixture(c *gc.C, errs ...error) *fixture {
	fix := &fixture{}
	fix.Stub.SetErrors(errs...)
	return fix
}

type testFunc func(*singular.FlagWorker, *testing.Clock, func())

func (fix *fixture) Run(c *gc.C, test testFunc) {
	facade := newStubFacade(&fix.Stub)
	clock := testing.NewClock(time.Now())
	flagWorker, err := singular.NewFlagWorker(singular.FlagConfig{
		Facade:   facade,
		Clock:    clock,
		Duration: time.Minute,
	})
	c.Assert(err, jc.ErrorIsNil)

	done := make(chan struct{})
	go func() {
		defer close(done)
		defer worker.Stop(flagWorker)
		defer facade.unblock()
		test(flagWorker, clock, facade.unblock)
	}()
	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("test timed out")
	}
}

func (fix *fixture) CheckClaimWait(c *gc.C) {
	fix.CheckCalls(c, []testing.StubCall{{
		FuncName: "Claim",
		Args:     []interface{}{time.Minute},
	}, {
		FuncName: "Wait",
	}})
}

func (fix *fixture) CheckClaims(c *gc.C, count int) {
	expect := make([]testing.StubCall, count)
	for i := 0; i < count; i++ {
		expect[i] = testing.StubCall{
			FuncName: "Claim",
			Args:     []interface{}{time.Minute},
		}
	}
	fix.CheckCalls(c, expect)
}

type stubFacade struct {
	stub  *testing.Stub
	mu    sync.Mutex
	block chan struct{}
}

func newStubFacade(stub *testing.Stub) *stubFacade {
	return &stubFacade{
		stub:  stub,
		block: make(chan struct{}),
	}
}

func (facade *stubFacade) unblock() {
	facade.mu.Lock()
	defer facade.mu.Unlock()
	select {
	case <-facade.block:
	default:
		close(facade.block)
	}
}

func (facade *stubFacade) Claim(duration time.Duration) error {
	facade.stub.AddCall("Claim", duration)
	return facade.stub.NextErr()
}

func (facade *stubFacade) Wait() error {
	facade.stub.AddCall("Wait")
	<-facade.block
	return facade.stub.NextErr()
}

var errClaimDenied = errors.Trace(lease.ErrClaimDenied)

type mockAgent struct {
	agent.Agent
	wrongKind bool
}

func (mock *mockAgent) CurrentConfig() agent.Config {
	return &mockAgentConfig{wrongKind: mock.wrongKind}
}

type mockAgentConfig struct {
	agent.Config
	wrongKind bool
}

func (mock *mockAgentConfig) Tag() names.Tag {
	if mock.wrongKind {
		return names.NewUnitTag("foo/1")
	}
	return names.NewMachineTag("123")
}

type fakeClock struct {
	clock.Clock
}

type fakeFacade struct {
	singular.Facade
}

type fakeWorker struct {
	worker.Worker
}

type fakeAPICaller struct {
	base.APICaller
}
