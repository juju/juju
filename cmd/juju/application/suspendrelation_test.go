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

type SuspendRelationSuite struct {
	testing.IsolationSuite
	mockAPI *mockSuspendAPI
}

func (s *SuspendRelationSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.mockAPI = &mockSuspendAPI{Stub: &testing.Stub{}, version: 6}
	s.mockAPI.setRelationSuspendedFunc = func(relationId int, suspended bool) error {
		return s.mockAPI.NextErr()
	}
}

var _ = gc.Suite(&SuspendRelationSuite{})

func (s *SuspendRelationSuite) runSuspendRelation(c *gc.C, args ...string) error {
	_, err := cmdtesting.RunCommand(c, NewSuspendRelationCommandForTest(s.mockAPI), args...)
	return err
}

func (s *SuspendRelationSuite) TestSuspendRelationWrongNumberOfArguments(c *gc.C) {
	// No arguments
	err := s.runSuspendRelation(c)
	c.Assert(err, gc.ErrorMatches, "no relation id specified")

	// 1 argument not an integer
	err = s.runSuspendRelation(c, "application1")
	c.Assert(err, gc.ErrorMatches, `relation ID "application1" not valid`)

	// More than 2 arguments
	err = s.runSuspendRelation(c, "123", "extra")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["extra"\]`)
}

func (s *SuspendRelationSuite) TestSuspendRelationIdOldServer(c *gc.C) {
	s.mockAPI.version = 4
	err := s.runSuspendRelation(c, "123")
	c.Assert(err, gc.ErrorMatches, "suspending a relation is not supported by this version of Juju")
	s.mockAPI.CheckCall(c, 0, "Close")
}

func (s *SuspendRelationSuite) TestSuspendRelationSuccess(c *gc.C) {
	err := s.runSuspendRelation(c, "123")
	c.Assert(err, jc.ErrorIsNil)
	s.mockAPI.CheckCall(c, 0, "SetRelationSuspended", 123, true)
	s.mockAPI.CheckCall(c, 1, "Close")
}

func (s *SuspendRelationSuite) TestSuspendRelationFail(c *gc.C) {
	msg := "fail suspend-relation at API"
	s.mockAPI.SetErrors(errors.New(msg))
	err := s.runSuspendRelation(c, "123")
	c.Assert(err, gc.ErrorMatches, msg)
	s.mockAPI.CheckCall(c, 0, "SetRelationSuspended", 123, true)
	s.mockAPI.CheckCall(c, 1, "Close")
}

func (s *SuspendRelationSuite) TestSuspendRelationBlocked(c *gc.C) {
	s.mockAPI.SetErrors(common.OperationBlockedError("TestSuspendRelationBlocked"))
	err := s.runSuspendRelation(c, "123")
	coretesting.AssertOperationWasBlocked(c, err, ".*TestSuspendRelationBlocked.*")
	s.mockAPI.CheckCall(c, 0, "SetRelationSuspended", 123, true)
	s.mockAPI.CheckCall(c, 1, "Close")
}

type mockSuspendAPI struct {
	*testing.Stub
	version                  int
	setRelationSuspendedFunc func(relationId int, suspended bool) error
}

func (s mockSuspendAPI) Close() error {
	s.MethodCall(s, "Close")
	return s.NextErr()
}

func (s mockSuspendAPI) SetRelationSuspended(relationId int, suspended bool) error {
	s.MethodCall(s, "SetRelationSuspended", relationId, suspended)
	return s.setRelationSuspendedFunc(relationId, suspended)
}

func (s mockSuspendAPI) BestAPIVersion() int {
	return s.version
}
