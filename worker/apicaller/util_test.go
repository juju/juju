// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apicaller_test

import (
	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
)

type mockAgent struct {
	agent.Agent
	stub *testing.Stub
	env  names.EnvironTag
}

func (mock *mockAgent) CurrentConfig() agent.Config {
	return dummyConfig{env: mock.env}
}

func (mock *mockAgent) ChangeConfig(mutator agent.ConfigMutator) error {
	mock.stub.AddCall("ChangeConfig", mutator)
	return mock.stub.NextErr()
}

type dummyConfig struct {
	agent.Config
	env names.EnvironTag
}

func (dummy dummyConfig) Environment() names.EnvironTag {
	return dummy.env
}

type mockSetter struct {
	stub *testing.Stub
	agent.ConfigSetter
}

func (mock *mockSetter) Migrate(params agent.MigrateParams) error {
	mock.stub.AddCall("Migrate", params)
	return mock.stub.NextErr()
}

type mockConn struct {
	stub *testing.Stub
	api.Connection
	broken chan struct{}
}

func (mock *mockConn) EnvironTag() (names.EnvironTag, error) {
	mock.stub.AddCall("EnvironTag")
	if err := mock.stub.NextErr(); err != nil {
		return names.EnvironTag{}, err
	}
	return coretesting.EnvironmentTag, nil
}

func (mock *mockConn) Broken() <-chan struct{} {
	return mock.broken
}

func (mock *mockConn) Close() error {
	mock.stub.AddCall("Close")
	return mock.stub.NextErr()
}

type mockGate struct {
	stub *testing.Stub
}

func (mock *mockGate) Unlock() {
	mock.stub.AddCall("Unlock")
}

type dummyWorker struct {
	worker.Worker
}

func assertStop(c *gc.C, w worker.Worker) {
	c.Assert(worker.Stop(w), jc.ErrorIsNil)
}

func assertStopError(c *gc.C, w worker.Worker, match string) {
	c.Assert(worker.Stop(w), gc.ErrorMatches, match)
}
