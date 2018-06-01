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
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	coretesting "github.com/juju/juju/testing"
)

type SuspendRelationSuite struct {
	testing.IsolationSuite
	mockAPI *mockSuspendAPI
}

func (s *SuspendRelationSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.mockAPI = &mockSuspendAPI{Stub: &testing.Stub{}, version: 6}
	s.mockAPI.setRelationSuspendedFunc = func(relationIds []int, suspended bool, message string) error {
		return s.mockAPI.NextErr()
	}
}

var _ = gc.Suite(&SuspendRelationSuite{})

func (s *SuspendRelationSuite) runSuspendRelation(c *gc.C, args ...string) error {
	store := jujuclienttesting.MinimalStore()
	_, err := cmdtesting.RunCommand(c, NewSuspendRelationCommandForTest(s.mockAPI, store), args...)
	return err
}

func (s *SuspendRelationSuite) TestSuspendRelationInvalidArguments(c *gc.C) {
	// No arguments
	err := s.runSuspendRelation(c)
	c.Assert(err, gc.ErrorMatches, "no relation ids specified")

	// argument not an integer
	err = s.runSuspendRelation(c, "application1")
	c.Assert(err, gc.ErrorMatches, `relation ID "application1" not valid`)
}

func (s *SuspendRelationSuite) TestSuspendRelationIdOldServer(c *gc.C) {
	s.mockAPI.version = 4
	err := s.runSuspendRelation(c, "123")
	c.Assert(err, gc.ErrorMatches, "suspending a relation is not supported by this version of Juju")
	s.mockAPI.CheckCall(c, 0, "Close")
}

func (s *SuspendRelationSuite) TestSuspendRelationSuccess(c *gc.C) {
	err := s.runSuspendRelation(c, "123", "456", "--message", "message")
	c.Assert(err, jc.ErrorIsNil)
	s.mockAPI.CheckCall(c, 0, "SetRelationSuspended", []int{123, 456}, true, "message")
	s.mockAPI.CheckCall(c, 1, "Close")
}

func (s *SuspendRelationSuite) TestSuspendRelationFail(c *gc.C) {
	msg := "fail suspend-relation at API"
	s.mockAPI.SetErrors(errors.New(msg))
	err := s.runSuspendRelation(c, "123")
	c.Assert(err, gc.ErrorMatches, msg)
	s.mockAPI.CheckCall(c, 0, "SetRelationSuspended", []int{123}, true, "")
	s.mockAPI.CheckCall(c, 1, "Close")
}

func (s *SuspendRelationSuite) TestSuspendRelationBlocked(c *gc.C) {
	s.mockAPI.SetErrors(common.OperationBlockedError("TestSuspendRelationBlocked"))
	err := s.runSuspendRelation(c, "123")
	coretesting.AssertOperationWasBlocked(c, err, ".*TestSuspendRelationBlocked.*")
	s.mockAPI.CheckCall(c, 0, "SetRelationSuspended", []int{123}, true, "")
	s.mockAPI.CheckCall(c, 1, "Close")
}

type mockSuspendAPI struct {
	*testing.Stub
	version                  int
	setRelationSuspendedFunc func(relationIds []int, suspended bool, message string) error
}

func (s mockSuspendAPI) Close() error {
	s.MethodCall(s, "Close")
	return s.NextErr()
}

func (s mockSuspendAPI) SetRelationSuspended(relationIds []int, suspended bool, message string) error {
	s.MethodCall(s, "SetRelationSuspended", relationIds, suspended, message)
	return s.setRelationSuspendedFunc(relationIds, suspended, message)
}

func (s mockSuspendAPI) BestAPIVersion() int {
	return s.version
}
