// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type LeaderGetSuite struct {
	jujutesting.IsolationSuite
}

var _ = gc.Suite(&LeaderGetSuite{})

func (s *LeaderGetSuite) TestInitError(c *gc.C) {
	command := jujuc.NewLeaderGetCommand(nil)
	err := command.Init([]string{"x=x"})
	c.Assert(err, gc.ErrorMatches, `invalid key "x=x"`)
}

func (s *LeaderGetSuite) TestInitKey(c *gc.C) {
	command := jujuc.NewLeaderGetCommand(nil)
	err := command.Init([]string{"some-key"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *LeaderGetSuite) TestInitAll(c *gc.C) {
	command := jujuc.NewLeaderGetCommand(nil)
	err := command.Init([]string{"-"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *LeaderGetSuite) TestInitEmpty(c *gc.C) {
	command := jujuc.NewLeaderGetCommand(nil)
	err := command.Init(nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *LeaderGetSuite) TestFormatError(c *gc.C) {
	command := jujuc.NewLeaderGetCommand(nil)
	runContext := testing.Context(c)
	code := cmd.Main(command, runContext, []string{"--format", "bad"})
	c.Check(code, gc.Equals, 2)
	c.Check(bufferString(runContext.Stdout), gc.Equals, "")
	c.Check(bufferString(runContext.Stderr), gc.Equals, `error: invalid value "bad" for flag --format: unknown format "bad"`+"\n")
}

func (s *LeaderGetSuite) TestSettingsError(c *gc.C) {
	jujucContext := newLeaderGetContext(errors.New("zap"))
	command := jujuc.NewLeaderGetCommand(jujucContext)
	runContext := testing.Context(c)
	code := cmd.Main(command, runContext, nil)
	c.Check(code, gc.Equals, 1)
	c.Check(jujucContext.called, jc.IsTrue)
	c.Check(bufferString(runContext.Stdout), gc.Equals, "")
	c.Check(bufferString(runContext.Stderr), gc.Equals, "error: cannot read leadership settings: zap\n")
}

func (s *LeaderGetSuite) TestSettingsFormatDefaultMissingKey(c *gc.C) {
	s.testOutput(c, []string{"unknown"}, "")
}

func (s *LeaderGetSuite) TestSettingsFormatDefaultKey(c *gc.C) {
	s.testOutput(c, []string{"key"}, "value\n")
}

func (s *LeaderGetSuite) TestSettingsFormatDefaultAll(c *gc.C) {
	s.testParseOutput(c, []string{"-"}, jc.YAMLEquals, leaderGetSettings())
}

func (s *LeaderGetSuite) TestSettingsFormatDefaultEmpty(c *gc.C) {
	s.testParseOutput(c, nil, jc.YAMLEquals, leaderGetSettings())
}

func (s *LeaderGetSuite) TestSettingsFormatSmartMissingKey(c *gc.C) {
	s.testOutput(c, []string{"--format", "smart", "unknown"}, "")
}

func (s *LeaderGetSuite) TestSettingsFormatSmartKey(c *gc.C) {
	s.testOutput(c, []string{"--format", "smart", "key"}, "value\n")
}

func (s *LeaderGetSuite) TestSettingsFormatSmartAll(c *gc.C) {
	s.testParseOutput(c, []string{"--format", "smart", "-"}, jc.YAMLEquals, leaderGetSettings())
}

func (s *LeaderGetSuite) TestSettingsFormatSmartEmpty(c *gc.C) {
	s.testParseOutput(c, []string{"--format", "smart"}, jc.YAMLEquals, leaderGetSettings())
}

func (s *LeaderGetSuite) TestSettingsFormatJSONMissingKey(c *gc.C) {
	s.testParseOutput(c, []string{"--format", "json", "unknown"}, jc.JSONEquals, nil)
}

func (s *LeaderGetSuite) TestSettingsFormatJSONKey(c *gc.C) {
	s.testParseOutput(c, []string{"--format", "json", "key"}, jc.JSONEquals, "value")
}

func (s *LeaderGetSuite) TestSettingsFormatJSONAll(c *gc.C) {
	s.testParseOutput(c, []string{"--format", "json", "-"}, jc.JSONEquals, leaderGetSettings())
}

func (s *LeaderGetSuite) TestSettingsFormatJSONEmpty(c *gc.C) {
	s.testParseOutput(c, []string{"--format", "json"}, jc.JSONEquals, leaderGetSettings())
}

func (s *LeaderGetSuite) TestSettingsFormatYAMLMissingKey(c *gc.C) {
	s.testParseOutput(c, []string{"--format", "yaml", "unknown"}, jc.YAMLEquals, nil)
}

func (s *LeaderGetSuite) TestSettingsFormatYAMLKey(c *gc.C) {
	s.testParseOutput(c, []string{"--format", "yaml", "key"}, jc.YAMLEquals, "value")
}

func (s *LeaderGetSuite) TestSettingsFormatYAMLAll(c *gc.C) {
	s.testParseOutput(c, []string{"--format", "yaml", "-"}, jc.YAMLEquals, leaderGetSettings())
}

func (s *LeaderGetSuite) TestSettingsFormatYAMLEmpty(c *gc.C) {
	s.testParseOutput(c, []string{"--format", "yaml"}, jc.YAMLEquals, leaderGetSettings())
}

func (s *LeaderGetSuite) testOutput(c *gc.C, args []string, expect string) {
	s.testParseOutput(c, args, gc.Equals, expect)
}

func (s *LeaderGetSuite) testParseOutput(c *gc.C, args []string, checker gc.Checker, expect interface{}) {
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

func (c *leaderGetContext) LeaderSettings() (params.Settings, error) {
	c.called = true
	return c.settings, c.err
}
