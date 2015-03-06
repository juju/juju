// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type leaderGetSuite struct {
	jujutesting.IsolationSuite
}

var _ = gc.Suite(&leaderGetSuite{})

func (s *leaderGetSuite) TestInitError(c *gc.C) {
	command := jujuc.NewLeaderGetCommand(nil)
	err := command.Init([]string{"x=x"})
	c.Assert(err, gc.ErrorMatches, `invalid key "x=x"`)
}

func (s *leaderGetSuite) TestInitKey(c *gc.C) {
	command := jujuc.NewLeaderGetCommand(nil)
	err := command.Init([]string{"some-key"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *leaderGetSuite) TestInitAll(c *gc.C) {
	command := jujuc.NewLeaderGetCommand(nil)
	err := command.Init([]string{"-"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *leaderGetSuite) TestInitEmpty(c *gc.C) {
	command := jujuc.NewLeaderGetCommand(nil)
	err := command.Init(nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *leaderGetSuite) TestFormatError(c *gc.C) {
	command := jujuc.NewLeaderGetCommand(nil)
	runContext := testing.Context(c)
	code := cmd.Main(command, runContext, []string{"--format", "bad"})
	c.Check(code, gc.Equals, 2)
	c.Check(bufferString(runContext.Stdout), gc.Equals, "")
	c.Check(bufferString(runContext.Stderr), gc.Equals, `error: invalid value "bad" for flag --format: unknown format "bad"`+"\n")
}

func (s *leaderGetSuite) TestSettingsError(c *gc.C) {
	jujucContext := newLeaderGetContext(errors.New("zap"))
	command := jujuc.NewLeaderGetCommand(jujucContext)
	runContext := testing.Context(c)
	code := cmd.Main(command, runContext, nil)
	c.Check(code, gc.Equals, 1)
	c.Check(jujucContext.called, jc.IsTrue)
	c.Check(bufferString(runContext.Stdout), gc.Equals, "")
	c.Check(bufferString(runContext.Stderr), gc.Equals, "error: cannot read leadership settings: zap\n")
}

func (s *leaderGetSuite) TestSettingsFormatDefaultMissingKey(c *gc.C) {
	s.testOutput(c, []string{"unknown"}, "")
}

func (s *leaderGetSuite) TestSettingsFormatDefaultKey(c *gc.C) {
	s.testOutput(c, []string{"key"}, "value\n")
}

func (s *leaderGetSuite) TestSettingsFormatDefaultAll(c *gc.C) {
	s.testParseOutput(c, []string{"-"}, jc.YAMLEquals, leaderGetSettings())
}

func (s *leaderGetSuite) TestSettingsFormatDefaultEmpty(c *gc.C) {
	s.testParseOutput(c, nil, jc.YAMLEquals, leaderGetSettings())
}

func (s *leaderGetSuite) TestSettingsFormatSmartMissingKey(c *gc.C) {
	s.testOutput(c, []string{"--format", "smart", "unknown"}, "")
}

func (s *leaderGetSuite) TestSettingsFormatSmartKey(c *gc.C) {
	s.testOutput(c, []string{"--format", "smart", "key"}, "value\n")
}

func (s *leaderGetSuite) TestSettingsFormatSmartAll(c *gc.C) {
	s.testParseOutput(c, []string{"--format", "smart", "-"}, jc.YAMLEquals, leaderGetSettings())
}

func (s *leaderGetSuite) TestSettingsFormatSmartEmpty(c *gc.C) {
	s.testParseOutput(c, []string{"--format", "smart"}, jc.YAMLEquals, leaderGetSettings())
}

func (s *leaderGetSuite) TestSettingsFormatJSONMissingKey(c *gc.C) {
	s.testParseOutput(c, []string{"--format", "json", "unknown"}, jc.JSONEquals, nil)
}

func (s *leaderGetSuite) TestSettingsFormatJSONKey(c *gc.C) {
	s.testParseOutput(c, []string{"--format", "json", "key"}, jc.JSONEquals, "value")
}

func (s *leaderGetSuite) TestSettingsFormatJSONAll(c *gc.C) {
	s.testParseOutput(c, []string{"--format", "json", "-"}, jc.JSONEquals, leaderGetSettings())
}

func (s *leaderGetSuite) TestSettingsFormatJSONEmpty(c *gc.C) {
	s.testParseOutput(c, []string{"--format", "json"}, jc.JSONEquals, leaderGetSettings())
}

func (s *leaderGetSuite) TestSettingsFormatYAMLMissingKey(c *gc.C) {
	s.testParseOutput(c, []string{"--format", "yaml", "unknown"}, jc.YAMLEquals, nil)
}

func (s *leaderGetSuite) TestSettingsFormatYAMLKey(c *gc.C) {
	s.testParseOutput(c, []string{"--format", "yaml", "key"}, jc.YAMLEquals, "value")
}

func (s *leaderGetSuite) TestSettingsFormatYAMLAll(c *gc.C) {
	s.testParseOutput(c, []string{"--format", "yaml", "-"}, jc.YAMLEquals, leaderGetSettings())
}

func (s *leaderGetSuite) TestSettingsFormatYAMLEmpty(c *gc.C) {
	s.testParseOutput(c, []string{"--format", "yaml"}, jc.YAMLEquals, leaderGetSettings())
}

func (s *leaderGetSuite) testOutput(c *gc.C, args []string, expect string) {
	s.testParseOutput(c, args, gc.Equals, expect)
}

func (s *leaderGetSuite) testParseOutput(c *gc.C, args []string, checker gc.Checker, expect interface{}) {
	jujucContext := newLeaderGetContext(nil)
	command := jujuc.NewLeaderGetCommand(jujucContext)
	runContext := testing.Context(c)
	code := cmd.Main(command, runContext, args)
	c.Check(code, gc.Equals, 0)
	c.Check(jujucContext.called, jc.IsTrue)
	c.Check(bufferString(runContext.Stdout), checker, expect)
	c.Check(bufferString(runContext.Stderr), gc.Equals, "")
}

func leaderGetSettings() map[string]string {
	return map[string]string{
		"key":    "value",
		"sample": "settings",
	}
}

func newLeaderGetContext(err error) *leaderGetContext {
	if err != nil {
		return &leaderGetContext{err: err}
	}
	return &leaderGetContext{settings: leaderGetSettings()}
}

type leaderGetContext struct {
	jujuc.Context
	called   bool
	settings map[string]string
	err      error
}

func (c *leaderGetContext) LeaderSettings() (map[string]string, error) {
	c.called = true
	return c.settings, c.err
}
