// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for info.

package model_test

import (
	"context"
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/cmd/juju/model"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
)

type DumpCommandSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	fake  fakeDumpClient
	store *jujuclient.MemStore
}

func TestDumpCommandSuite(t *stdtesting.T) { tc.Run(t, &DumpCommandSuite{}) }

type fakeDumpClient struct {
	testhelpers.Stub
}

func (f *fakeDumpClient) Close() error {
	f.MethodCall(f, "Close")
	return f.NextErr()
}

func (f *fakeDumpClient) DumpModel(ctx context.Context, model names.ModelTag) (map[string]interface{}, error) {
	f.MethodCall(f, "DumpModel", model)
	err := f.NextErr()
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"model-uuid": "fake uuid",
	}, nil
}

func (s *DumpCommandSuite) SetUpTest(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
	s.store.Models["testing"].CurrentModel = "admin/mymodel"
}

func (s *DumpCommandSuite) TestDump(c *tc.C) {
	ctx, err := cmdtesting.RunCommand(c, model.NewDumpCommandForTest(&s.fake, s.store))
	c.Assert(err, tc.ErrorIsNil)
	s.fake.CheckCalls(c, []testhelpers.StubCall{
		{FuncName: "DumpModel", Args: []interface{}{testing.ModelTag}},
		{FuncName: "Close", Args: nil},
	})

	out := cmdtesting.Stdout(ctx)
	c.Assert(out, tc.Equals, "model-uuid: fake uuid\n")
}
