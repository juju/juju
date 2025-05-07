// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for info.

package model_test

import (
	"context"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/cmd/juju/model"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
)

type DumpDBCommandSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	fake  fakeDumpDBClient
	store *jujuclient.MemStore
}

var _ = tc.Suite(&DumpDBCommandSuite{})

func (s *DumpDBCommandSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.fake.ResetCalls()
	s.store = jujuclient.NewMemStore()
	s.store.CurrentControllerName = "testing"
	s.store.Controllers["testing"] = jujuclient.ControllerDetails{}
	s.store.Accounts["testing"] = jujuclient.AccountDetails{
		User: "admin",
	}
	err := s.store.UpdateModel("testing", "admin/mymodel", jujuclient.ModelDetails{
		ModelUUID: testing.ModelTag.Id(),
		ModelType: coremodel.IAAS,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.store.Models["testing"].CurrentModel = "admin/mymodel"
}

func (s *DumpDBCommandSuite) TestDumpDB(c *tc.C) {
	ctx, err := cmdtesting.RunCommand(c, model.NewDumpDBCommandForTest(&s.fake, s.store))
	c.Assert(err, jc.ErrorIsNil)
	s.fake.CheckCalls(c, []jujutesting.StubCall{
		{"DumpModelDB", []interface{}{testing.ModelTag}},
		{"Close", nil},
	})

	out := cmdtesting.Stdout(ctx)
	c.Assert(out, tc.Equals, `all-others: heaps of data
models:
  name: testing
  uuid: fake-uuid
`)
}

type fakeDumpDBClient struct {
	jujutesting.Stub
}

func (f *fakeDumpDBClient) Close() error {
	f.MethodCall(f, "Close")
	return f.NextErr()
}

func (f *fakeDumpDBClient) DumpModelDB(ctx context.Context, model names.ModelTag) (map[string]interface{}, error) {
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
