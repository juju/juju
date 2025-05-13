// Copyright 2015-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apicaller_test

import (
	"context"
	"net/url"
	"time"

	"github.com/juju/names/v6"
	"github.com/juju/retry"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	apiagent "github.com/juju/juju/api/agent/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/apicaller"
	"github.com/juju/juju/rpc/params"
)

var errNotProvisioned = &params.Error{Code: params.CodeNotProvisioned}
var errNotAuthorized = &params.Error{Code: params.CodeUnauthorized}

type mockAgent struct {
	agent.Agent
	stub   *testhelpers.Stub
	entity names.Tag
	model  names.ModelTag
}

func (mock *mockAgent) CurrentConfig() agent.Config {
	return dummyConfig{
		entity: mock.entity,
		model:  mock.model,
	}
}

func (mock *mockAgent) ChangeConfig(mutator agent.ConfigMutator) error {
	mock.stub.AddCall("ChangeConfig")
	if err := mock.stub.NextErr(); err != nil {
		return err
	}
	return mutator(&mockSetter{stub: mock.stub})
}

type dummyConfig struct {
	agent.Config
	entity names.Tag
	model  names.ModelTag
}

func (dummy dummyConfig) Tag() names.Tag {
	return dummy.entity
}

func (dummy dummyConfig) Model() names.ModelTag {
	return dummy.model
}

func (dummy dummyConfig) APIInfo() (*api.Info, bool) {
	return &api.Info{
		ModelTag: dummy.model,
		Tag:      dummy.entity,
		Password: "new",
	}, true
}

func (dummy dummyConfig) OldPassword() string {
	return "old"
}

type mockSetter struct {
	stub *testhelpers.Stub
	agent.ConfigSetter
}

func (mock *mockSetter) SetOldPassword(pw string) {
	mock.stub.AddCall("SetOldPassword", pw)
	mock.stub.PopNoErr()
}

func (mock *mockSetter) SetPassword(pw string) {
	mock.stub.AddCall("SetPassword", pw)
	mock.stub.PopNoErr()
}

type mockConn struct {
	stub *testhelpers.Stub
	api.Connection
	controllerOnly bool
	broken         chan struct{}
}

func (mock *mockConn) ModelTag() (names.ModelTag, bool) {
	mock.stub.AddCall("ModelTag")
	if mock.controllerOnly {
		return names.ModelTag{}, false
	}
	return coretesting.ModelTag, true
}

func (mock *mockConn) Broken() <-chan struct{} {
	return mock.broken
}

func (mock *mockConn) Addr() *url.URL {
	return &url.URL{
		Scheme: "wss",
		Host:   "testing.invalid",
	}
}

func (mock *mockConn) Close() error {
	mock.stub.AddCall("Close")
	return mock.stub.NextErr()
}

func newMockConnFacade(stub *testhelpers.Stub, life apiagent.Life) apiagent.ConnFacade {
	return &mockConnFacade{
		stub: stub,
		life: life,
	}
}

type mockConnFacade struct {
	stub *testhelpers.Stub
	life apiagent.Life
}

func (mock *mockConnFacade) Life(_ context.Context, entity names.Tag) (apiagent.Life, error) {
	mock.stub.AddCall("Life", entity)
	if err := mock.stub.NextErr(); err != nil {
		return "", err
	}
	return mock.life, nil
}

func (mock *mockConnFacade) SetPassword(_ context.Context, entity names.Tag, password string) error {
	mock.stub.AddCall("SetPassword", entity, password)
	return mock.stub.NextErr()
}

type dummyWorker struct {
	worker.Worker
}

func assertStop(c *tc.C, w worker.Worker) {
	c.Assert(worker.Stop(w), tc.ErrorIsNil)
}

func assertStopError(c *tc.C, w worker.Worker, match string) {
	c.Assert(worker.Stop(w), tc.ErrorMatches, match)
}

func lifeTest(c *tc.C, stub *testhelpers.Stub, life apiagent.Life, test func() (api.Connection, error)) (api.Connection, error) {
	newFacade := func(apiCaller base.APICaller) (apiagent.ConnFacade, error) {
		c.Check(apiCaller, tc.FitsTypeOf, (*mockConn)(nil))
		return newMockConnFacade(stub, life), nil
	}
	unpatch := testhelpers.PatchValue(apicaller.NewConnFacade, newFacade)
	defer unpatch()
	return test()
}

func strategyTest(stub *testhelpers.Stub, strategy retry.CallArgs, test func(api.OpenFunc) (api.Connection, error)) (api.Connection, error) {
	unpatch := testhelpers.PatchValue(apicaller.Strategy, strategy)
	defer unpatch()
	return test(func(ctx context.Context, info *api.Info, opts api.DialOpts) (api.Connection, error) {
		// copy because I don't trust what might happen to info
		stub.AddCall("apiOpen", *info, opts)
		err := stub.NextErr()
		if err != nil {
			return nil, err
		}
		return &mockConn{stub: stub}, nil
	})
}

func checkOpenCalls(c *tc.C, stub *testhelpers.Stub, passwords ...string) {
	calls := openCalls(names.ModelTag{}, testEntity, passwords...)
	stub.CheckCalls(c, calls)
}

func openCalls(model names.ModelTag, entity names.Tag, passwords ...string) []testhelpers.StubCall {
	calls := make([]testhelpers.StubCall, len(passwords))
	for i, pw := range passwords {
		info := api.Info{
			ModelTag: model,
			Tag:      entity,
			Password: pw,
		}
		calls[i] = testhelpers.StubCall{
			FuncName: "apiOpen",
			Args: []interface{}{info, api.DialOpts{
				DialAddressInterval: 200 * time.Millisecond,
				DialTimeout:         3 * time.Second,
				Timeout:             time.Minute,
			}},
		}
	}
	return calls
}
