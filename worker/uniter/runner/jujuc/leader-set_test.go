// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type leaderSetSuite struct {
	jujucSuite
	command cmd.Command
}

var _ = gc.Suite(&leaderSetSuite{})

func (s *leaderSetSuite) SetUpTest(c *gc.C) {
	var err error
	s.command, err = jujuc.NewLeaderSetCommand(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.command = jujuc.NewJujucCommandWrappedForTest(s.command)
}

func (s *leaderSetSuite) TestInitEmpty(c *gc.C) {
	err := s.command.Init(nil)
	c.Check(err, jc.ErrorIsNil)
}

func (s *leaderSetSuite) TestInitValues(c *gc.C) {
	err := s.command.Init([]string{"foo=bar", "baz=qux"})
	c.Check(err, jc.ErrorIsNil)
}

func (s *leaderSetSuite) TestInitError(c *gc.C) {
	err := s.command.Init([]string{"nonsense"})
	c.Check(err, gc.ErrorMatches, `expected "key=value", got "nonsense"`)
}

func (s *leaderSetSuite) TestWriteEmpty(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectWrite(map[string]string{}, nil)
	command, err := jujuc.NewLeaderSetCommand(s.mockContext)
	c.Assert(err, jc.ErrorIsNil)
	runContext := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(command), runContext, nil)
	c.Check(code, gc.Equals, 0)
	c.Check(bufferString(runContext.Stdout), gc.Equals, "")
	c.Check(bufferString(runContext.Stderr), gc.Equals, "")
}

func (s *leaderSetSuite) TestWriteValues(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectWrite(map[string]string{
		"foo": "bar",
		"baz": "qux",
	}, nil)
	command, err := jujuc.NewLeaderSetCommand(s.mockContext)
	c.Assert(err, jc.ErrorIsNil)
	runContext := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(command), runContext, []string{"foo=bar", "baz=qux"})
	c.Check(code, gc.Equals, 0)
	c.Check(bufferString(runContext.Stdout), gc.Equals, "")
	c.Check(bufferString(runContext.Stderr), gc.Equals, "")
}

func (s *leaderSetSuite) TestWriteError(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectWrite(map[string]string{"foo": "bar"}, errors.New("splat"))
	command, err := jujuc.NewLeaderSetCommand(s.mockContext)
	c.Assert(err, jc.ErrorIsNil)
	runContext := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(command), runContext, []string{"foo=bar"})
	c.Check(code, gc.Equals, 1)
	c.Check(bufferString(runContext.Stdout), gc.Equals, "")
	c.Check(bufferString(runContext.Stderr), gc.Equals, "ERROR cannot write leadership settings: splat\n")
}

func (s *leaderSetSuite) expectWrite(arg map[string]string, err error) {
	s.mockContext.EXPECT().WriteLeaderSettings(arg).Return(err)
}
