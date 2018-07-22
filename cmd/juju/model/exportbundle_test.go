// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for info.

package model_test

import (
	"github.com/juju/cmd/cmdtesting"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/model"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type ExportBundleCommandSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	fake  fakeExportBundleClient
	store *jujuclient.MemStore
}

var _ = gc.Suite(&ExportBundleCommandSuite{})

type fakeExportBundleClient struct {
	jujutesting.Stub
	bestAPIVersion int
}

func (f *fakeExportBundleClient) Close() error {
	f.MethodCall(f, "Close")
	return f.NextErr()
}

func (f *fakeExportBundleClient) ExportBundle() (string, error) {
	f.MethodCall(f, "ExportBundle", nil)
	return testing.ModelTag.String(), f.NextErr()
}

func (s *ExportBundleCommandSuite) SetUpTest(c *gc.C) {
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

func (s *ExportBundleCommandSuite) TestExportBundle(c *gc.C) {
	ctx, err := cmdtesting.RunCommand(c, model.NewExportBundleCommandForTest(&s.fake, s.store))
	c.Assert(err, jc.ErrorIsNil)
	s.fake.CheckCalls(c, []jujutesting.StubCall{
		{"ExportBundle", []interface{}{nil}},
		{"Close", nil},
	})

	out := cmdtesting.Stdout(ctx)
	c.Assert(out, gc.Equals, "model-deadbeef-0bad-400d-8000-4b1d0d06f00d.yaml\n")
}
