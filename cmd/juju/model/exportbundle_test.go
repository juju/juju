// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for info.

package model_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"

	"github.com/juju/cmd/cmdtesting"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	appFacade "github.com/juju/juju/apiserver/facades/client/application"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/model"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type ExportBundleCommandSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	fakeBundle *fakeExportBundleClient
	fakeConfig *fakeConfigClient
	stub       *jujutesting.Stub
	store      *jujuclient.MemStore
}

var _ = gc.Suite(&ExportBundleCommandSuite{})

func (s *ExportBundleCommandSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.stub = &jujutesting.Stub{}
	s.fakeBundle = &fakeExportBundleClient{
		Stub:           s.stub,
		bestAPIVersion: 3,
	}
	s.fakeConfig = &fakeConfigClient{
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
	if s.fakeBundle.filename != "" {
		err := os.Remove(s.fakeBundle.filename + ".yaml")
		if !os.IsNotExist(err) {
			c.Check(err, jc.ErrorIsNil)
		}
		err = os.Remove(s.fakeBundle.filename)
		if !os.IsNotExist(err) {
			c.Check(err, jc.ErrorIsNil)
		}
	}

	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
}

func (s *ExportBundleCommandSuite) TestExportBundleSuccessNoFilename(c *gc.C) {
	s.fakeBundle.result = "applications:\n" +
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

	ctx, err := cmdtesting.RunCommand(c, model.NewExportBundleCommandForTest(s.fakeBundle, s.fakeConfig, s.store))
	c.Assert(err, jc.ErrorIsNil)
	s.fakeBundle.CheckCalls(c, []jujutesting.StubCall{
		{"ExportBundle", []interface{}{false}},
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
	s.fakeBundle.filename = filepath.Join(c.MkDir(), "mymodel")
	s.fakeBundle.result = "applications:\n" +
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
	ctx, err := cmdtesting.RunCommand(c, model.NewExportBundleCommandForTest(s.fakeBundle, s.fakeConfig, s.store), "--filename", s.fakeBundle.filename)
	c.Assert(err, jc.ErrorIsNil)
	s.fakeBundle.CheckCalls(c, []jujutesting.StubCall{
		{"ExportBundle", []interface{}{false}},
	})

	out := cmdtesting.Stdout(ctx)
	c.Assert(out, gc.Equals, fmt.Sprintf("Bundle successfully exported to %s\n", s.fakeBundle.filename))
	output, err := ioutil.ReadFile(s.fakeBundle.filename)
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
	_, err := cmdtesting.RunCommand(c, model.NewExportBundleCommandForTest(s.fakeBundle, s.fakeConfig, s.store), "--filename")
	c.Assert(err, gc.NotNil)

	c.Assert(err.Error(), gc.Equals, "option needs an argument: --filename")
}

func (s *ExportBundleCommandSuite) TestExportBundleSuccesssOverwriteFilename(c *gc.C) {
	s.fakeBundle.filename = filepath.Join(c.MkDir(), "mymodel")
	s.fakeBundle.result = "fake-data"
	ctx, err := cmdtesting.RunCommand(c, model.NewExportBundleCommandForTest(s.fakeBundle, s.fakeConfig, s.store), "--filename", s.fakeBundle.filename)
	c.Assert(err, jc.ErrorIsNil)
	s.fakeBundle.CheckCalls(c, []jujutesting.StubCall{
		{"ExportBundle", []interface{}{false}},
	})

	out := cmdtesting.Stdout(ctx)
	c.Assert(out, gc.Equals, fmt.Sprintf("Bundle successfully exported to %s\n", s.fakeBundle.filename))
	output, err := ioutil.ReadFile(s.fakeBundle.filename)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(string(output), gc.Equals, "fake-data")
}

func (s *ExportBundleCommandSuite) TestExportBundleIncludeCharmDefaults(c *gc.C) {
	s.fakeBundle.filename = filepath.Join(c.MkDir(), "mymodel")
	s.fakeBundle.result = "fake-data"
	ctx, err := cmdtesting.RunCommand(c, model.NewExportBundleCommandForTest(s.fakeBundle, s.fakeConfig, s.store), "--include-charm-defaults", "--filename", s.fakeBundle.filename)
	c.Assert(err, jc.ErrorIsNil)
	s.fakeBundle.CheckCalls(c, []jujutesting.StubCall{
		{"ExportBundle", []interface{}{true}},
	})

	out := cmdtesting.Stdout(ctx)
	c.Assert(out, gc.Equals, fmt.Sprintf("Bundle successfully exported to %s\n", s.fakeBundle.filename))
	output, err := ioutil.ReadFile(s.fakeBundle.filename)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(string(output), gc.Equals, "fake-data")
}

func (s *ExportBundleCommandSuite) TestPatchOfExportedBundleToExposeTrustFlag(c *gc.C) {
	s.fakeBundle.result = "applications:\n" +
		"  aws-integrator:\n" +
		"    charm: cs:~containers/aws-integrator\n" +
		"    num_units: 1\n" +
		"    to:\n" +
		"    - \"0\"\n" +
		"  gcp-integrator:\n" +
		"    charm: cs:~containers/gcp-integrator\n" +
		"    num_units: 2\n" +
		"    to:\n" +
		"    - \"0\"\n" +
		"    - \"1\"\n" +
		"  ubuntu-lite:\n" +
		"    charm: cs:~jameinel/ubuntu-lite-7\n" +
		"    num_units: 1\n" +
		"    to:\n" +
		"    - \"2\"\n" +
		"machines:\n" +
		"  \"0\": {}\n" +
		"  \"1\": {}\n" +
		"  \"2\": {}\n" +
		"series: bionic\n"

	// Pretend we target an older controller that does not expose the
	// TrustRequired flag in the yaml output.
	s.fakeBundle.bestAPIVersion = 2

	s.fakeConfig.result = map[string]*params.ApplicationGetResults{
		"aws-integrator": {
			ApplicationConfig: map[string]interface{}{
				appFacade.TrustConfigOptionName: map[string]interface{}{
					"description": "Does this application have access to trusted credentials",
					"default":     false,
					"type":        "bool",
					"value":       true,
				},
			},
		},
		"gcp-integrator": {
			ApplicationConfig: map[string]interface{}{
				appFacade.TrustConfigOptionName: map[string]interface{}{
					"description": "Does this application have access to trusted credentials",
					"default":     false,
					"type":        "bool",
					"value":       false,
				},
			},
		},
		"ubuntu-lite": {
			ApplicationConfig: map[string]interface{}{
				"unrelated": map[string]interface{}{
					"description": "The question to the meaning of life",
					"default":     42,
					"type":        "int",
					"value":       42,
				},
			},
		},
	}

	ctx, err := cmdtesting.RunCommand(c, model.NewExportBundleCommandForTest(s.fakeBundle, s.fakeConfig, s.store))
	c.Assert(err, jc.ErrorIsNil)

	// Since we are iterating a map with applications we cannot call
	// stub.CheckCalls but instead need to collect the calls, sort them and
	// run the checks manually.
	var exportBundleCallCount, getConfigCallCount int
	var getConfigAppList []string
	for _, call := range s.stub.Calls() {
		switch call.FuncName {
		case "ExportBundle":
			exportBundleCallCount++
		case "Get":
			getConfigCallCount++
			getConfigAppList = append(getConfigAppList, call.Args[1].(string))
		default:
			c.Fatalf("unexpected call to %q", call.FuncName)
		}
	}
	sort.Strings(getConfigAppList)

	c.Assert(exportBundleCallCount, gc.Equals, 1)
	c.Assert(getConfigCallCount, gc.Equals, 3)
	c.Assert(getConfigAppList, gc.DeepEquals, []string{"aws-integrator", "gcp-integrator", "ubuntu-lite"})

	out := cmdtesting.Stdout(ctx)
	c.Assert(out, gc.Equals, ""+
		"applications:\n"+
		"  aws-integrator:\n"+
		"    charm: cs:~containers/aws-integrator\n"+
		"    num_units: 1\n"+
		"    to:\n"+
		"    - \"0\"\n"+
		"    trust: true\n"+
		"  gcp-integrator:\n"+
		"    charm: cs:~containers/gcp-integrator\n"+
		"    num_units: 2\n"+
		"    to:\n"+
		"    - \"0\"\n"+
		"    - \"1\"\n"+
		"  ubuntu-lite:\n"+
		"    charm: cs:~jameinel/ubuntu-lite-7\n"+
		"    num_units: 1\n"+
		"    to:\n"+
		"    - \"2\"\n"+
		"machines:\n"+
		"  \"0\": {}\n"+
		"  \"1\": {}\n"+
		"  \"2\": {}\n"+
		"series: bionic\n")
}

type fakeExportBundleClient struct {
	*jujutesting.Stub
	result         string
	filename       string
	bestAPIVersion int
}

func (f *fakeExportBundleClient) BestAPIVersion() int { return f.bestAPIVersion }

func (f *fakeExportBundleClient) Close() error { return nil }

func (f *fakeExportBundleClient) ExportBundle(includeDefaults bool) (string, error) {
	f.MethodCall(f, "ExportBundle", includeDefaults)
	if err := f.NextErr(); err != nil {
		return "", err
	}

	return f.result, f.NextErr()
}

type fakeConfigClient struct {
	*jujutesting.Stub
	result map[string]*params.ApplicationGetResults
}

func (f *fakeConfigClient) Close() error { return nil }

func (f *fakeConfigClient) Get(branch string, app string) (*params.ApplicationGetResults, error) {
	f.MethodCall(f, "Get", branch, app)
	if err := f.NextErr(); err != nil {
		return nil, err
	}

	if f.result == nil {
		return nil, f.NextErr()
	}

	return f.result[app], f.NextErr()
}
