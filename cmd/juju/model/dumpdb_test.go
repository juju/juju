// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for info.

package model_test

import (
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cmd/juju/model"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/testing"
)

type DumpDBCommandSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	fake  fakeDumpDBClient
	store *jujuclienttesting.MemStore
}

var _ = gc.Suite(&DumpDBCommandSuite{})

func (s *DumpDBCommandSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.fake.ResetCalls()
	s.store = jujuclienttesting.NewMemStore()
	s.store.CurrentControllerName = "testing"
	s.store.Controllers["testing"] = jujuclient.ControllerDetails{}
	s.store.Accounts["testing"] = jujuclient.AccountDetails{
		User: "admin@local",
	}
	err := s.store.UpdateModel("testing", "admin@local/mymodel", jujuclient.ModelDetails{
		testing.ModelTag.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)
	s.store.Models["testing"].CurrentModel = "admin@local/mymodel"
}

func (s *DumpDBCommandSuite) TestDumpDB(c *gc.C) {
	ctx, err := testing.RunCommand(c, model.NewDumpDBCommandForTest(&s.fake, s.store))
	c.Assert(err, jc.ErrorIsNil)
	s.fake.CheckCalls(c, []gitjujutesting.StubCall{
		{"DumpModelDB", []interface{}{testing.ModelTag}},
		{"Close", nil},
	})

	out := testing.Stdout(ctx)
	c.Assert(out, gc.Equals, `all-others: heaps of data
models:
  name: testing
  uuid: fake-uuid
`)
}

type fakeDumpDBClient struct {
	gitjujutesting.Stub
}

func (f *fakeDumpDBClient) Close() error {
	f.MethodCall(f, "Close")
	return f.NextErr()
}

func (f *fakeDumpDBClient) DumpModelDB(model names.ModelTag) (map[string]interface{}, error) {
	f.MethodCall(f, "DumpModelDB", model)
	err := f.NextErr()
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"models": map[string]interface{}{
			"name": "testing",
			"uuid": "fake-uuid",
		},
		"all-others": "heaps of data",
	}, nil
}
