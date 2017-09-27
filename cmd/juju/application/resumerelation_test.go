// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	coretesting "github.com/juju/juju/testing"
)

type ResumeRelationSuite struct {
	testing.IsolationSuite
	mockAPI *mockResumeAPI
}

func (s *ResumeRelationSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.mockAPI = &mockResumeAPI{Stub: &testing.Stub{}, version: 6}
	s.mockAPI.setRelationSuspendedFunc = func(relationId int, suspended bool) error {
		return s.mockAPI.NextErr()
	}
}

var _ = gc.Suite(&ResumeRelationSuite{})

func (s *ResumeRelationSuite) runResumeRelation(c *gc.C, args ...string) error {
	_, err := cmdtesting.RunCommand(c, NewResumeRelationCommandForTest(s.mockAPI), args...)
	return err
}

func (s *ResumeRelationSuite) TestResumeRelationWrongNumberOfArguments(c *gc.C) {
	// No arguments
	err := s.runResumeRelation(c)
	c.Assert(err, gc.ErrorMatches, "no relation id specified")

	// 1 argument not an integer
	err = s.runResumeRelation(c, "application1")
	c.Assert(err, gc.ErrorMatches, `relation ID "application1" not valid`)

	// More than 2 arguments
	err = s.runResumeRelation(c, "123", "extra")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["extra"\]`)
}

func (s *ResumeRelationSuite) TestResumeRelationIdOldServer(c *gc.C) {
	s.mockAPI.version = 4
	err := s.runResumeRelation(c, "123")
	c.Assert(err, gc.ErrorMatches, "resuming a relation is not supported by this version of Juju")
	s.mockAPI.CheckCall(c, 0, "Close")
}

func (s *ResumeRelationSuite) TestResumeRelationSuccess(c *gc.C) {
	err := s.runResumeRelation(c, "123")
	c.Assert(err, jc.ErrorIsNil)
	s.mockAPI.CheckCall(c, 0, "SetRelationSuspended", 123, false)
	s.mockAPI.CheckCall(c, 1, "Close")
}

func (s *ResumeRelationSuite) TestResumeRelationFail(c *gc.C) {
	msg := "fail resume-relation at API"
	s.mockAPI.SetErrors(errors.New(msg))
	err := s.runResumeRelation(c, "123")
	c.Assert(err, gc.ErrorMatches, msg)
	s.mockAPI.CheckCall(c, 0, "SetRelationSuspended", 123, false)
	s.mockAPI.CheckCall(c, 1, "Close")
}

func (s *ResumeRelationSuite) TestResumeRelationBlocked(c *gc.C) {
	s.mockAPI.SetErrors(common.OperationBlockedError("TestResumeRelationBlocked"))
	err := s.runResumeRelation(c, "123")
	coretesting.AssertOperationWasBlocked(c, err, ".*TestResumeRelationBlocked.*")
	s.mockAPI.CheckCall(c, 0, "SetRelationSuspended", 123, false)
	s.mockAPI.CheckCall(c, 1, "Close")
}

type mockResumeAPI struct {
	*testing.Stub
	version                  int
	setRelationSuspendedFunc func(relationId int, suspended bool) error
}

func (s mockResumeAPI) Close() error {
	s.MethodCall(s, "Close")
	return s.NextErr()
}

func (s mockResumeAPI) SetRelationSuspended(relationId int, suspended bool) error {
	s.MethodCall(s, "SetRelationSuspended", relationId, suspended)
	return s.setRelationSuspendedFunc(relationId, suspended)
}

func (s mockResumeAPI) BestAPIVersion() int {
	return s.version
}
