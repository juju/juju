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

type leaderSetSuite struct {
	jujutesting.IsolationSuite
}

var _ = gc.Suite(&leaderSetSuite{})

func (s *leaderSetSuite) TestInitEmpty(c *gc.C) {
	command := jujuc.NewLeaderSetCommand(nil)
	err := command.Init(nil)
	c.Check(err, jc.ErrorIsNil)
}

func (s *leaderSetSuite) TestInitValues(c *gc.C) {
	command := jujuc.NewLeaderSetCommand(nil)
	err := command.Init([]string{"foo=bar", "baz=qux"})
	c.Check(err, jc.ErrorIsNil)
}

func (s *leaderSetSuite) TestInitError(c *gc.C) {
	command := jujuc.NewLeaderSetCommand(nil)
	err := command.Init([]string{"nonsense"})
	c.Check(err, gc.ErrorMatches, `expected "key=value", got "nonsense"`)
}

func (s *leaderSetSuite) TestWriteEmpty(c *gc.C) {
	jujucContext := &leaderSetContext{}
	command := jujuc.NewLeaderSetCommand(jujucContext)
	runContext := testing.Context(c)
	code := cmd.Main(command, runContext, nil)
	c.Check(code, gc.Equals, 0)
	c.Check(jujucContext.gotSettings, jc.DeepEquals, map[string]string{})
	c.Check(bufferString(runContext.Stdout), gc.Equals, "")
	c.Check(bufferString(runContext.Stderr), gc.Equals, "")
}

func (s *leaderSetSuite) TestWriteValues(c *gc.C) {
	jujucContext := &leaderSetContext{}
	command := jujuc.NewLeaderSetCommand(jujucContext)
	runContext := testing.Context(c)
	code := cmd.Main(command, runContext, []string{"foo=bar", "baz=qux"})
	c.Check(code, gc.Equals, 0)
	c.Check(jujucContext.gotSettings, jc.DeepEquals, map[string]string{
		"foo": "bar",
		"baz": "qux",
	})
	c.Check(bufferString(runContext.Stdout), gc.Equals, "")
	c.Check(bufferString(runContext.Stderr), gc.Equals, "")
}

func (s *leaderSetSuite) TestWriteError(c *gc.C) {
	jujucContext := &leaderSetContext{err: errors.New("splat")}
	command := jujuc.NewLeaderSetCommand(jujucContext)
	runContext := testing.Context(c)
	code := cmd.Main(command, runContext, []string{"foo=bar"})
	c.Check(code, gc.Equals, 1)
	c.Check(jujucContext.gotSettings, jc.DeepEquals, map[string]string{"foo": "bar"})
	c.Check(bufferString(runContext.Stdout), gc.Equals, "")
	c.Check(bufferString(runContext.Stderr), gc.Equals, "error: cannot write leadership settings: splat\n")
}

type leaderSetContext struct {
	jujuc.Context
	gotSettings map[string]string
	err         error
}

func (s *leaderSetContext) WriteLeaderSettings(settings map[string]string) error {
	s.gotSettings = settings
	return s.err
}
