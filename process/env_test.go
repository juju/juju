// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package process_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/process"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&envSuite{})

type envSuite struct {
	testing.BaseSuite
}

func (*envSuite) TestParseEnvOkay(c *gc.C) {
	raw := []string{"A=1", "B=2", "C=3"}
	env, err := process.ParseEnv(raw)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(env, jc.DeepEquals, map[string]string{
		"A": "1",
		"B": "2",
		"C": "3",
	})
}

func (*envSuite) TestParseEnvEmpty(c *gc.C) {
	var raw []string
	env, err := process.ParseEnv(raw)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(env, gc.HasLen, 0)
}

func (*envSuite) TestParseEnvEssentiallyEmpty(c *gc.C) {
	raw := []string{""}
	env, err := process.ParseEnv(raw)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(env, gc.HasLen, 0)
}

func (*envSuite) TestParseEnvSkipped(c *gc.C) {
	raw := []string{"A=1", "B=2", "", "D=4"}
	env, err := process.ParseEnv(raw)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(env, jc.DeepEquals, map[string]string{
		"A": "1",
		"B": "2",
		"D": "4",
	})
}

func (*envSuite) TestParseEnvMissing(c *gc.C) {
	raw := []string{"A=1", "B=", "C", "D=4"}
	env, err := process.ParseEnv(raw)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(env, jc.DeepEquals, map[string]string{
		"A": "1",
		"B": "",
		"C": "",
		"D": "4",
	})
}

func (*envSuite) TestParseEnvBadName(c *gc.C) {
	raw := []string{"=1"}
	_, err := process.ParseEnv(raw)

	c.Check(err, gc.ErrorMatches, `got "" for env var name`)
}

func (*envSuite) TestUnparseEnvOkay(c *gc.C) {
	env := map[string]string{
		"A": "1",
		"B": "2",
		"C": "3",
	}
	raw, err := process.UnparseEnv(env)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(raw, jc.DeepEquals, []string{"A=1", "B=2", "C=3"})
}

func (*envSuite) TestUnparseEnvEmpty(c *gc.C) {
	var env map[string]string
	raw, err := process.UnparseEnv(env)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(raw, gc.IsNil)
}

func (*envSuite) TestUnparseEnvMissingKey(c *gc.C) {
	env := map[string]string{
		"A": "1",
		"":  "2",
		"C": "3",
	}
	_, err := process.UnparseEnv(env)

	c.Check(err, gc.ErrorMatches, `got "" for env var name`)
}

func (*envSuite) TestUnparseEnvMissing(c *gc.C) {
	env := map[string]string{
		"A": "1",
		"B": "",
		"C": "3",
	}
	raw, err := process.UnparseEnv(env)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(raw, jc.DeepEquals, []string{"A=1", "B=", "C=3"})
}
