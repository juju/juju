// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for info.

package model_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

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

func (s *ExportBundleCommandSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.stub = &jujutesting.Stub{}
	s.fake = &fakeExportBundleClient{
		Stub: s.stub,
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

func (s *ExportBundleCommandSuite) TearDownTest(c *gc.C) {
	if s.fake.filename != "" {
		err := os.Remove(s.fake.filename + ".yaml")
		if !os.IsNotExist(err) {
			c.Check(err, jc.ErrorIsNil)
		}
	}

	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
}

func (s *ExportBundleCommandSuite) TestExportBundleSuccessNoFilename(c *gc.C) {
	s.fake.result = "applications:\n" +
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
		"  - mysql:mysql\n"

	ctx, err := cmdtesting.RunCommand(c, model.NewExportBundleCommandForTest(s.fake, s.store))
	c.Assert(err, jc.ErrorIsNil)
	s.fake.CheckCalls(c, []jujutesting.StubCall{
		{"ExportBundle", nil},
	})

	out := cmdtesting.Stdout(ctx)
	c.Assert(out, gc.Equals, ""+
		"applications:\n"+
		"  mysql:\n"+
		"    charm: \"\"\n"+
		"    num_units: 1\n"+
		"    to:\n"+
		"    - \"0\"\n"+
		"  wordpress:\n"+
		"    charm: \"\"\n"+
		"    num_units: 2\n"+
		"    to:\n"+
		"    - \"0\"\n"+
		"    - \"1\"\n"+
		"machines:\n"+
		"  \"0\": {}\n"+
		"  \"1\": {}\n"+
		"series: xenial\n"+
		"relations:\n"+
		"- - wordpress:db\n"+
		"  - mysql:mysql\n")
}

func (s *ExportBundleCommandSuite) TestExportBundleSuccessFilename(c *gc.C) {
	s.fake.filename = filepath.Join(c.MkDir(), "mymodel")
	s.fake.result = "applications:\n" +
		"  magic:\n" +
		"    charm: cs:zesty/magic\n" +
		"    series: zesty\n" +
		"    expose: true\n" +
		"    options:\n" +
		"      key: value\n" +
		"    bindings:\n" +
		"      rel-name: some-space\n" +
		"series: xenial\n" +
		"relations:\n" +
		"- []\n"
	ctx, err := cmdtesting.RunCommand(c, model.NewExportBundleCommandForTest(s.fake, s.store), "--filename", s.fake.filename)
	c.Assert(err, jc.ErrorIsNil)
	s.fake.CheckCalls(c, []jujutesting.StubCall{
		{"ExportBundle", nil},
	})

	out := cmdtesting.Stdout(ctx)
	c.Assert(out, gc.Equals, fmt.Sprintf("Bundle successfully exported to %s\n", s.fake.filename))
	output, err := ioutil.ReadFile(s.fake.filename)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(string(output), gc.Equals, "applications:\n"+
		"  magic:\n"+
		"    charm: cs:zesty/magic\n"+
		"    series: zesty\n"+
		"    expose: true\n"+
		"    options:\n"+
		"      key: value\n"+
		"    bindings:\n"+
		"      rel-name: some-space\n"+
		"series: xenial\n"+
		"relations:\n"+
		"- []\n")
}

func (s *ExportBundleCommandSuite) TestExportBundleFailNoFilename(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, model.NewExportBundleCommandForTest(s.fake, s.store), "--filename")
	c.Assert(err, gc.NotNil)

	c.Assert(err.Error(), gc.Equals, "flag needs an argument: --filename")
}

func (s *ExportBundleCommandSuite) TestExportBundleSuccesssOverwriteFilename(c *gc.C) {
	s.fake.filename = filepath.Join(c.MkDir(), "mymodel")
	s.fake.result = "fake-data"
	ctx, err := cmdtesting.RunCommand(c, model.NewExportBundleCommandForTest(s.fake, s.store), "--filename", s.fake.filename)
	c.Assert(err, jc.ErrorIsNil)
	s.fake.CheckCalls(c, []jujutesting.StubCall{
		{"ExportBundle", nil},
	})

	out := cmdtesting.Stdout(ctx)
	c.Assert(out, gc.Equals, fmt.Sprintf("Bundle successfully exported to %s\n", s.fake.filename))
	output, err := ioutil.ReadFile(s.fake.filename)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(string(output), gc.Equals, "fake-data")
}

func (f *fakeExportBundleClient) Close() error { return nil }

func (f *fakeExportBundleClient) ExportBundle() (string, error) {
	f.MethodCall(f, "ExportBundle")
	if err := f.NextErr(); err != nil {
		return "", err
	}

	return f.result, f.NextErr()
}

type fakeExportBundleClient struct {
	*jujutesting.Stub
	result   string
	filename string
}
