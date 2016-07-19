// Copyright 2015 Canonical Ltd.
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

type DumpCommandSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	fake  fakeDumpClient
	store *jujuclienttesting.MemStore
}

var _ = gc.Suite(&DumpCommandSuite{})

type fakeDumpClient struct {
	gitjujutesting.Stub
}

func (f *fakeDumpClient) Close() error {
	f.MethodCall(f, "Close")
	return f.NextErr()
}

func (f *fakeDumpClient) DumpModel(model names.ModelTag) ([]byte, error) {
	f.MethodCall(f, "DumpModel", model)
	err := f.NextErr()
	if err != nil {
		return nil, err
	}
	return []byte("dump model result"), nil
}

func (s *DumpCommandSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.fake.ResetCalls()
	s.store = jujuclienttesting.NewMemStore()
	s.store.CurrentControllerName = "testing"
	s.store.Controllers["testing"] = jujuclient.ControllerDetails{}
	s.store.Accounts["testing"] = jujuclient.AccountDetails{
		User: "admin@local",
	}
	err := s.store.UpdateModel("testing", "mymodel", jujuclient.ModelDetails{
		testing.ModelTag.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)
	s.store.Models["testing"].CurrentModel = "mymodel"
}

func (s *DumpCommandSuite) TestDump(c *gc.C) {
	ctx, err := testing.RunCommand(c, model.NewDumpCommandForTest(&s.fake, s.store))
	c.Assert(err, jc.ErrorIsNil)
	s.fake.CheckCalls(c, []gitjujutesting.StubCall{
		{"DumpModel", []interface{}{testing.ModelTag}},
		{"Close", nil},
	})

	out := testing.Stdout(ctx)
	c.Assert(out, gc.Equals, "dump model result\n")
}
