// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package process_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/process"
	"github.com/juju/juju/testing"
)

type pluginSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&pluginSuite{})

func (*pluginSuite) TestParseEnvOkay(c *gc.C) {
	raw := []string{"A=1", "B=2", "C=3"}
	env := process.ParseEnv(raw)

	c.Check(env, jc.DeepEquals, map[string]string{
		"A": "1",
		"B": "2",
		"C": "3",
	})
}

func (*pluginSuite) TestParseEnvEmpty(c *gc.C) {
	var raw []string
	env := process.ParseEnv(raw)

	c.Check(env, gc.IsNil)
}

func (*pluginSuite) TestParseEnvSkipped(c *gc.C) {
	raw := []string{"A=1", "B=2", "", "D=4"}
	env := process.ParseEnv(raw)

	c.Check(env, jc.DeepEquals, map[string]string{
		"A": "1",
		"B": "2",
		"D": "4",
	})
}

func (*pluginSuite) TestParseEnvMissing(c *gc.C) {
	raw := []string{"A=1", "B=", "C", "D=4"}
	env := process.ParseEnv(raw)

	c.Check(env, jc.DeepEquals, map[string]string{
		"A": "1",
		"B": "",
		"C": "",
		"D": "4",
	})
}

func (*pluginSuite) TestUnparseEnvOkay(c *gc.C) {
	env := map[string]string{
		"A": "1",
		"B": "2",
		"C": "3",
	}
	raw := process.UnparseEnv(env)

	c.Check(raw, jc.DeepEquals, []string{"A=1", "B=2", "C=3"})
}

func (*pluginSuite) TestUnparseEnvEmpty(c *gc.C) {
	var raw map[string]string
	env := process.UnparseEnv(raw)

	c.Check(env, gc.IsNil)
}

func (*pluginSuite) TestUnparseEnvMissingKey(c *gc.C) {
	env := map[string]string{
		"A": "1",
		"":  "2",
		"C": "3",
	}
	raw := process.UnparseEnv(env)

	c.Check(raw, jc.DeepEquals, []string{"A=1", "=2", "C=3"})
}

func (*pluginSuite) TestUnparseEnvMissing(c *gc.C) {
	env := map[string]string{
		"A": "1",
		"B": "",
		"C": "3",
	}
	raw := process.UnparseEnv(env)

	c.Check(raw, jc.DeepEquals, []string{"A=1", "B=", "C=3"})
}
