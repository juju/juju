// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package plugin_test

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/plugin"
	"github.com/juju/juju/workload/plugin/docker"
	"github.com/juju/juju/workload/plugin/executable"
)

type pluginSuite struct {
	testing.IsolationSuite

	stub *testing.Stub
}

var _ = gc.Suite(&pluginSuite{})

func (s *pluginSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
}

func createExecutable(c *gc.C, name, script string) {
	pluginDir := c.MkDir()
	filename := filepath.Join(pluginDir, "juju-workload-"+name)
	err := ioutil.WriteFile(filename, []byte(script), 0755)
	c.Assert(err, jc.ErrorIsNil)
	err = os.Setenv("PATH", pluginDir)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *pluginSuite) TestFindTestingPlugin(c *gc.C) {
	dataDir := c.MkDir()
	createExecutable(c, "testing-plugin", "#...")

	plugin, err := plugin.Find("testing-plugin", dataDir)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(plugin, gc.FitsTypeOf, &executable.Plugin{})
}

func (s *pluginSuite) TestFindNotTestingPlugin(c *gc.C) {
	dataDir := c.MkDir()

	plugin, err := plugin.Find("docker", dataDir)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(plugin, gc.FitsTypeOf, &docker.Plugin{})
}

func (s *pluginSuite) TestFindExecutableOkay(c *gc.C) {
	stub := stubFindFuncs{stub: s.stub}
	expected := &executable.Plugin{}
	stub.plugin = expected

	plugin, err := plugin.FindPlugin("docker", stub.findExecutable, stub.findBuiltin)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(plugin, gc.Equals, expected)
	s.stub.CheckCallNames(c, "findExecutable")
}

func (s *pluginSuite) TestFindExecutableError(c *gc.C) {
	stub := stubFindFuncs{stub: s.stub}
	failure := errors.Errorf("<failed>")
	s.stub.SetErrors(failure)

	_, err := plugin.FindPlugin("docker", stub.findExecutable, stub.findBuiltin)

	c.Check(errors.Cause(err), gc.Equals, failure)
	s.stub.CheckCallNames(c, "findExecutable")
}

func (s *pluginSuite) TestFindBuiltinOkay(c *gc.C) {
	stub := stubFindFuncs{stub: s.stub}
	expected := &docker.Plugin{}
	stub.plugin = expected
	notFound := errors.NotFoundf("docker")
	s.stub.SetErrors(notFound)

	plugin, err := plugin.FindPlugin("docker", stub.findExecutable, stub.findBuiltin)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(plugin, gc.Equals, expected)
	s.stub.CheckCallNames(c, "findExecutable", "findBuiltin")
}

func (s *pluginSuite) TestFindBuiltinError(c *gc.C) {
	stub := stubFindFuncs{stub: s.stub}
	failure := errors.Errorf("<failed>")
	notFound := errors.NotFoundf("docker")
	s.stub.SetErrors(notFound, failure)

	_, err := plugin.FindPlugin("docker", stub.findExecutable, stub.findBuiltin)

	c.Check(errors.Cause(err), gc.Equals, failure)
	s.stub.CheckCallNames(c, "findExecutable", "findBuiltin")
}

func (s *pluginSuite) TestFindNotFound(c *gc.C) {
	stub := stubFindFuncs{stub: s.stub}
	notFound := errors.NotFoundf("docker")
	s.stub.SetErrors(notFound, notFound)

	_, err := plugin.FindPlugin("docker", stub.findExecutable, stub.findBuiltin)

	c.Check(err, jc.Satisfies, errors.IsNotFound)
	s.stub.CheckCallNames(c, "findExecutable", "findBuiltin")
}

type stubFindFuncs struct {
	stub *testing.Stub

	plugin workload.Plugin
}

func (s *stubFindFuncs) findExecutable(name string) (workload.Plugin, error) {
	s.stub.AddCall("findExecutable", name)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.plugin, nil
}

func (s *stubFindFuncs) findBuiltin(name string) (workload.Plugin, error) {
	s.stub.AddCall("findBuiltin", name)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.plugin, nil
}
