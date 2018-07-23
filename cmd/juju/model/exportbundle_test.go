// Copyright 2018 Canonical Ltd.
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
	fake  *fakeExportBundleClient
	stub  *jujutesting.Stub
	store *jujuclient.MemStore
}

var _ = gc.Suite(&ExportBundleCommandSuite{})

type fakeExportBundleClient struct {
	*jujutesting.Stub
	bestAPIVersion int
}

func (f *fakeExportBundleClient) Close() error { return nil }

func (f *fakeExportBundleClient) ExportBundle() (string, error) {
	f.MethodCall(f, "ExportBundle")
	if err := f.NextErr(); err != nil {
		return "", err
	}
	return "applications:\n" +
		"  mysql:\n" +
		"    charm: \"\"\n" +
		"    num_units: 1\n" +
		"    to:\n" +
		"    - \"0\"\n" +
		"  wordpress:\n" +
		"    charm: \"\"\n" +
		"    num_units: 2\n" +
		"    to:\n" +
		"    - \"0\"\n" +
		"    - \"1\"\n" +
		"machines:\n" +
		"  \"0\": {}\n" +
		"  \"1\": {}\n" +
		"series: xenial\n" +
		"relations:\n" +
		"- - wordpress:db\n" +
		"  - mysql:mysql\n", f.NextErr()
}

func (f *fakeExportBundleClient) BestAPIVersion() int {
	return f.bestAPIVersion
}

func (s *ExportBundleCommandSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.stub = &jujutesting.Stub{}
	s.fake = &fakeExportBundleClient{
		Stub:           s.stub,
		bestAPIVersion: 2,
	}
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

func (s *ExportBundleCommandSuite) TestExportBundleNoFilename(c *gc.C) {
	ctx, err := cmdtesting.RunCommand(c, model.NewExportBundleCommandForTest(s.fake, s.store))
	c.Assert(err, jc.ErrorIsNil)
	s.fake.CheckCalls(c, []jujutesting.StubCall{
		{"ExportBundle", nil},
	})

	out := cmdtesting.Stdout(ctx)
	c.Assert(out, gc.Equals, "|\n"+
		"  applications:\n"+
		"    mysql:\n"+
		"      charm: \"\"\n"+
		"      num_units: 1\n"+
		"      to:\n"+
		"      - \"0\"\n"+
		"    wordpress:\n"+
		"      charm: \"\"\n"+
		"      num_units: 2\n"+
		"      to:\n"+
		"      - \"0\"\n"+
		"      - \"1\"\n"+
		"  machines:\n"+
		"    \"0\": {}\n"+
		"    \"1\": {}\n"+
		"  series: xenial\n"+
		"  relations:\n"+
		"  - - wordpress:db\n"+
		"    - mysql:mysql\n")
}
