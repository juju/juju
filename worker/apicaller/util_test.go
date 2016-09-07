// Copyright 2015-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apicaller_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	apiagent "github.com/juju/juju/api/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/apicaller"
)

var errNotProvisioned = &params.Error{Code: params.CodeNotProvisioned}
var errNotAuthorized = &params.Error{Code: params.CodeUnauthorized}

type mockAgent struct {
	agent.Agent
	stub   *testing.Stub
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
	stub *testing.Stub
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
	stub *testing.Stub
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

func (mock *mockConn) Close() error {
	mock.stub.AddCall("Close")
	return mock.stub.NextErr()
}

func newMockConnFacade(stub *testing.Stub, life apiagent.Life) apiagent.ConnFacade {
	return &mockConnFacade{
		stub: stub,
		life: life,
	}
}

type mockConnFacade struct {
	stub *testing.Stub
	life apiagent.Life
}

func (mock *mockConnFacade) Life(entity names.Tag) (apiagent.Life, error) {
	mock.stub.AddCall("Life", entity)
	if err := mock.stub.NextErr(); err != nil {
		return "", err
	}
	return mock.life, nil
}

func (mock *mockConnFacade) SetPassword(entity names.Tag, password string) error {
	mock.stub.AddCall("SetPassword", entity, password)
	return mock.stub.NextErr()
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

func lifeTest(c *gc.C, stub *testing.Stub, life apiagent.Life, test func() (api.Connection, error)) (api.Connection, error) {
	newFacade := func(apiCaller base.APICaller) (apiagent.ConnFacade, error) {
		c.Check(apiCaller, gc.FitsTypeOf, (*mockConn)(nil))
		return newMockConnFacade(stub, life), nil
	}
	unpatch := testing.PatchValue(apicaller.NewConnFacade, newFacade)
	defer unpatch()
	return test()
}

// TODO(katco): 2016-08-09: lp:1611427
func strategyTest(stub *testing.Stub, strategy utils.AttemptStrategy, test func(api.OpenFunc) (api.Connection, error)) (api.Connection, error) {
	unpatch := testing.PatchValue(apicaller.Strategy, strategy)
	defer unpatch()
	return test(func(info *api.Info, opts api.DialOpts) (api.Connection, error) {
		// copy because I don't trust what might happen to info
		stub.AddCall("apiOpen", *info, opts)
		err := stub.NextErr()
		if err != nil {
			return nil, err
		}
		return &mockConn{stub: stub}, nil
	})
}

func checkOpenCalls(c *gc.C, stub *testing.Stub, passwords ...string) {
	calls := openCalls(names.ModelTag{}, nil, passwords...)
	stub.CheckCalls(c, calls)
}

func openCalls(model names.ModelTag, entity names.Tag, passwords ...string) []testing.StubCall {
	calls := make([]testing.StubCall, len(passwords))
	for i, pw := range passwords {
		info := api.Info{
			ModelTag: model,
			Tag:      entity,
			Password: pw,
		}
		calls[i] = testing.StubCall{
			FuncName: "apiOpen",
			Args:     []interface{}{info, api.DialOpts{}},
		}
	}
	return calls
}
