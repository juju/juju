// Copyright 2012 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"time"

	"github.com/juju/cmd/v4/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	coretesting "github.com/juju/juju/testing"
)

type RemoveRelationSuite struct {
	testing.IsolationSuite
	mockAPI *mockRemoveAPI
}

func (s *RemoveRelationSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.mockAPI = &mockRemoveAPI{Stub: &testing.Stub{}}
	s.mockAPI.removeRelationFunc = func(force *bool, maxWait *time.Duration, endpoints ...string) error {
		return s.mockAPI.NextErr()
	}
}

var _ = gc.Suite(&RemoveRelationSuite{})

func (s *RemoveRelationSuite) runRemoveRelation(c *gc.C, args ...string) error {
	store := jujuclienttesting.MinimalStore()
	_, err := cmdtesting.RunCommand(c, NewRemoveRelationCommandForTest(s.mockAPI, store), args...)
	return err
}

func (s *RemoveRelationSuite) TestRemoveRelationWrongNumberOfArguments(c *gc.C) {
	// No arguments
	err := s.runRemoveRelation(c)
	c.Assert(err, gc.ErrorMatches, "a relation must involve two applications")

	// 1 argument not an integer
	err = s.runRemoveRelation(c, "application1")
	c.Assert(err, gc.ErrorMatches, `relation ID "application1" not valid`)

	// More than 2 arguments
	err = s.runRemoveRelation(c, "application1", "application2", "application3")
	c.Assert(err, gc.ErrorMatches, "a relation must involve two applications")
}

func (s *RemoveRelationSuite) TestRemoveRelationSuccess(c *gc.C) {
	err := s.runRemoveRelation(c, "application1", "application2")
	c.Assert(err, jc.ErrorIsNil)
	s.mockAPI.CheckCall(c, 0, "DestroyRelation", (*bool)(nil), (*time.Duration)(nil), []string{"application1", "application2"})
	s.mockAPI.CheckCall(c, 1, "Close")
}

func (s *RemoveRelationSuite) TestRemoveRelationIdSuccess(c *gc.C) {
	err := s.runRemoveRelation(c, "123")
	c.Assert(err, jc.ErrorIsNil)
	s.mockAPI.CheckCall(c, 0, "DestroyRelationId", 123, (*bool)(nil), (*time.Duration)(nil))
	s.mockAPI.CheckCall(c, 1, "Close")
}

func (s *RemoveRelationSuite) TestRemoveRelationFail(c *gc.C) {
	msg := "fail remove-relation at API"
	s.mockAPI.SetErrors(errors.New(msg))
	err := s.runRemoveRelation(c, "application1", "application2")
	c.Assert(err, gc.ErrorMatches, msg)
	s.mockAPI.CheckCall(c, 0, "DestroyRelation", (*bool)(nil), (*time.Duration)(nil), []string{"application1", "application2"})
	s.mockAPI.CheckCall(c, 1, "Close")
}

func (s *RemoveRelationSuite) TestRemoveRelationBlocked(c *gc.C) {
	s.mockAPI.SetErrors(apiservererrors.OperationBlockedError("TestRemoveRelationBlocked"))
	err := s.runRemoveRelation(c, "application1", "application2")
	coretesting.AssertOperationWasBlocked(c, err, ".*TestRemoveRelationBlocked.*")
	s.mockAPI.CheckCall(c, 0, "DestroyRelation", (*bool)(nil), (*time.Duration)(nil), []string{"application1", "application2"})
	s.mockAPI.CheckCall(c, 1, "Close")
}

type mockRemoveAPI struct {
	*testing.Stub
	removeRelationFunc func(force *bool, maxWait *time.Duration, endpoints ...string) error
}

func (s mockRemoveAPI) Close() error {
	s.MethodCall(s, "Close")
	return s.NextErr()
}

func (s mockRemoveAPI) DestroyRelation(force *bool, maxWait *time.Duration, endpoints ...string) error {
	s.MethodCall(s, "DestroyRelation", force, maxWait, endpoints)
	return s.removeRelationFunc(force, maxWait, endpoints...)
}

func (s mockRemoveAPI) DestroyRelationId(relationId int, force *bool, maxWait *time.Duration) error {
	s.MethodCall(s, "DestroyRelationId", relationId, force, maxWait)
	return nil
}
