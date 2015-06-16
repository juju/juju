// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package process_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/process"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&LaunchDetailsSuite{})
var _ = gc.Suite(&pluginSuite{})

type LaunchDetailsSuite struct {
	testing.BaseSuite
}

func (*LaunchDetailsSuite) TestIsZeroTrue(c *gc.C) {
	details := process.LaunchDetails{}
	isZero := details.IsZero()

	c.Check(isZero, jc.IsTrue)
}

func (*LaunchDetailsSuite) TestIsZeroFalse(c *gc.C) {
	details := process.LaunchDetails{
		ID:     "abc123",
		Status: "running",
	}
	isZero := details.IsZero()

	c.Check(isZero, jc.IsFalse)
}

type pluginSuite struct {
	testing.BaseSuite
}

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

func (*pluginSuite) TestParseDetailsValid(c *gc.C) {
	input := `{"id":"1234", "status":"running"}`

	ld, err := process.ParseDetails(input)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(ld, jc.DeepEquals, &process.LaunchDetails{
		ID:     "1234",
		Status: "running",
	})
}

func (*pluginSuite) TestParseDetailsMissingID(c *gc.C) {
	input := `{"status":"running"}`

	_, err := process.ParseDetails(input)
	c.Assert(err, gc.ErrorMatches, "ID must be set")
}

func (*pluginSuite) TestParseDetailsMissingStatus(c *gc.C) {
	input := `{"id":"1234"}`

	_, err := process.ParseDetails(input)
	c.Assert(err, gc.ErrorMatches, "Status must be set")
}

func (*pluginSuite) TestParseDetailsExtraInfo(c *gc.C) {
	input := `{"id":"1234", "status":"running", "extra":"stuff"}`

	ld, err := process.ParseDetails(input)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(ld, jc.DeepEquals, &process.LaunchDetails{
		ID:     "1234",
		Status: "running",
	})
}
