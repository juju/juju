// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus_test

import (
	"runtime"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/keyvalues"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/meterstatus"
)

type ContextSuite struct{}

var _ = gc.Suite(&ContextSuite{})

type dummyPaths struct{}

func (*dummyPaths) GetToolsDir() string             { return "/dummy/tools" }
func (*dummyPaths) GetCharmDir() string             { return "/dummy/charm" }
func (*dummyPaths) GetJujucSocket() string          { return "/dummy/jujuc.sock" }
func (*dummyPaths) GetMetricsSpoolDir() string      { return "/dummy/spool" }
func (*dummyPaths) ComponentDir(name string) string { return "/dummy/" + name }

func (s *ContextSuite) TestHookContextEnv(c *gc.C) {
	ctx := meterstatus.NewLimitedContext("u/0")
	paths := &dummyPaths{}
	vars, err := ctx.HookVars(paths)
	c.Assert(err, jc.ErrorIsNil)
	varMap, err := keyvalues.Parse(vars, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(varMap["JUJU_AGENT_SOCKET"], gc.Equals, "/dummy/jujuc.sock")
	c.Assert(varMap["JUJU_UNIT_NAME"], gc.Equals, "u/0")
	key := "PATH"
	if runtime.GOOS == "windows" {
		key = "Path"
	}
	c.Assert(varMap[key], gc.Not(gc.Equals), "")
}

func (s *ContextSuite) TestHookContextSetEnv(c *gc.C) {
	ctx := meterstatus.NewLimitedContext("u/0")
	setVars := map[string]string{
		"somekey":    "somevalue",
		"anotherkey": "anothervalue",
	}
	ctx.SetEnvVars(setVars)
	paths := &dummyPaths{}
	vars, err := ctx.HookVars(paths)
	c.Assert(err, jc.ErrorIsNil)
	varMap, err := keyvalues.Parse(vars, true)
	c.Assert(err, jc.ErrorIsNil)
	for key, value := range setVars {
		c.Assert(varMap[key], gc.Equals, value)
	}
	c.Assert(varMap["JUJU_AGENT_SOCKET"], gc.Equals, "/dummy/jujuc.sock")
	c.Assert(varMap["JUJU_UNIT_NAME"], gc.Equals, "u/0")
}
