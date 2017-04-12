// Copyright 2012 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cmd/cmdtesting"
	coretesting "github.com/juju/juju/testing"
)

type RemoveRelationSuite struct {
	testing.IsolationSuite
	mockAPI *mockRemoveAPI
}

func (s *RemoveRelationSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.mockAPI = &mockRemoveAPI{Stub: &testing.Stub{}}
	s.mockAPI.removeRelationFunc = func(endpoints ...string) error {
		return s.mockAPI.NextErr()
	}
}

var _ = gc.Suite(&RemoveRelationSuite{})

func (s *RemoveRelationSuite) runRemoveRelation(c *gc.C, args ...string) error {
	_, err := cmdtesting.RunCommand(c, NewRemoveRelationCommandForTest(s.mockAPI), args...)
	return err
}

func (s *RemoveRelationSuite) TestRemoveRelationWrongNumberOfArguments(c *gc.C) {
	// No arguments
	err := s.runRemoveRelation(c)
	c.Assert(err, gc.ErrorMatches, "a relation must involve two applications")

	// 1 argument
	err = s.runRemoveRelation(c, "application1")
	c.Assert(err, gc.ErrorMatches, "a relation must involve two applications")

	// More than 2 arguments
	err = s.runRemoveRelation(c, "application1", "application2", "application3")
	c.Assert(err, gc.ErrorMatches, "a relation must involve two applications")
}

func (s *RemoveRelationSuite) TestRemoveRelationSuccess(c *gc.C) {
	err := s.runRemoveRelation(c, "application1", "application2")
	c.Assert(err, jc.ErrorIsNil)
	s.mockAPI.CheckCall(c, 0, "DestroyRelation", []string{"application1", "application2"})
	s.mockAPI.CheckCall(c, 1, "Close")
}

func (s *RemoveRelationSuite) TestRemoveRelationFail(c *gc.C) {
	msg := "fail remove-relation at API"
	s.mockAPI.SetErrors(errors.New(msg))
	err := s.runRemoveRelation(c, "application1", "application2")
	c.Assert(err, gc.ErrorMatches, msg)
	s.mockAPI.CheckCall(c, 0, "DestroyRelation", []string{"application1", "application2"})
	s.mockAPI.CheckCall(c, 1, "Close")
}

func (s *RemoveRelationSuite) TestRemoveRelationBlocked(c *gc.C) {
	s.mockAPI.SetErrors(common.OperationBlockedError("TestRemoveRelationBlocked"))
	err := s.runRemoveRelation(c, "application1", "application2")
	coretesting.AssertOperationWasBlocked(c, err, ".*TestRemoveRelationBlocked.*")
	s.mockAPI.CheckCall(c, 0, "DestroyRelation", []string{"application1", "application2"})
	s.mockAPI.CheckCall(c, 1, "Close")
}

type mockRemoveAPI struct {
	*testing.Stub
	removeRelationFunc func(endpoints ...string) error
}

func (s mockRemoveAPI) Close() error {
	s.MethodCall(s, "Close")
	return s.NextErr()
}

func (s mockRemoveAPI) DestroyRelation(endpoints ...string) error {
	s.MethodCall(s, "DestroyRelation", endpoints)
	return s.removeRelationFunc(endpoints...)
}
