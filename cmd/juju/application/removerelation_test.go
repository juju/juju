// Copyright 2012 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	coretesting "github.com/juju/juju/testing"
)

type RemoveRelationSuite struct {
	testing.IsolationSuite
	mockAPI *mockRemoveAPI
}

func (s *RemoveRelationSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.mockAPI = &mockRemoveAPI{
		removeRelationFunc: func(endpoints ...string) error {
			return nil
		},
	}
}

var _ = gc.Suite(&RemoveRelationSuite{})

func (s *RemoveRelationSuite) runRemoveRelation(c *gc.C, args ...string) error {
	_, err := coretesting.RunCommand(c, NewRemoveRelationCommandForTest(s.mockAPI), args...)
	return err
}

func (s *RemoveRelationSuite) TestRemoveRelationNotEnoughArguments(c *gc.C) {
	// No arguments
	err := s.runRemoveRelation(c)
	c.Assert(err, gc.ErrorMatches, "a relation must involve two applications")

	// 1 empty argument
	err = s.runRemoveRelation(c, "")
	c.Assert(err, gc.ErrorMatches, "a relation must involve two applications")

	// 2 empty arguments
	err = s.runRemoveRelation(c, "", "")
	c.Assert(err, gc.ErrorMatches, "a relation must involve two applications")

	// 1 empty and 1 non-empty arguments
	err = s.runRemoveRelation(c, "application1", "")
	c.Assert(err, gc.ErrorMatches, "a relation must involve two applications")

	// 1 empty and 1 non-empty arguments
	err = s.runRemoveRelation(c, "", "application2")
	c.Assert(err, gc.ErrorMatches, "a relation must involve two applications")
}

func (s *RemoveRelationSuite) TestRemoveRelationSuccess(c *gc.C) {
	err := s.runRemoveRelation(c, "application1", "application2")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *RemoveRelationSuite) TestRemoveRelationFail(c *gc.C) {
	msg := "fail remove relation"
	s.mockAPI = &mockRemoveAPI{
		removeRelationFunc: func(endpoints ...string) error {
			return errors.New(msg)
		},
	}
	err := s.runRemoveRelation(c, "application1", "application2")
	c.Assert(err, gc.ErrorMatches, msg)
}

func (s *RemoveRelationSuite) TestRemoveRelationBlocked(c *gc.C) {
	// Block operation
	s.mockAPI.removeRelationFunc = func(endpoints ...string) error {
		return common.OperationBlockedError("TestRemoveRelationBlocked")
	}
	err := s.runRemoveRelation(c, "application1", "application2")
	coretesting.AssertOperationWasBlocked(c, err, ".*TestRemoveRelationBlocked.*")
}

type mockRemoveAPI struct {
	removeRelationFunc func(endpoints ...string) error
}

func (s mockRemoveAPI) Close() error {
	return nil
}

func (s mockRemoveAPI) DestroyRelation(endpoints ...string) error {
	return s.removeRelationFunc(endpoints...)
}
