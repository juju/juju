// Copyright 2012 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
)

type AddRelationSuite struct {
	testing.IsolationSuite
	mockAPI *mockAddAPI
}

func (s *AddRelationSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.mockAPI = &mockAddAPI{Stub: &testing.Stub{}}
	s.mockAPI.addRelationFunc = func(endpoints ...string) (*params.AddRelationResults, error) {
		// At the moment, cmd implementation ignores the return values,
		// so nil is an acceptable return for testing purposes.
		return nil, s.mockAPI.NextErr()
	}
}

var _ = gc.Suite(&AddRelationSuite{})

func (s *AddRelationSuite) runAddRelation(c *gc.C, args ...string) error {
	_, err := coretesting.RunCommand(c, NewAddRelationCommandForTest(s.mockAPI), args...)
	return err
}

func (s *AddRelationSuite) TestAddRelationWrongNumberOfArguments(c *gc.C) {
	// No arguments
	err := s.runAddRelation(c)
	c.Assert(err, gc.ErrorMatches, "a relation must involve two applications")

	// 1 argument
	err = s.runAddRelation(c, "application1")
	c.Assert(err, gc.ErrorMatches, "a relation must involve two applications")

	// more than 2 arguments
	err = s.runAddRelation(c, "application1", "application2", "application3")
	c.Assert(err, gc.ErrorMatches, "a relation must involve two applications")
}

func (s *AddRelationSuite) TestAddRelationSuccess(c *gc.C) {
	err := s.runAddRelation(c, "application1", "application2")
	c.Assert(err, jc.ErrorIsNil)
	s.mockAPI.CheckCallNames(c, "AddRelation", "Close")
}

func (s *AddRelationSuite) TestAddRelationFail(c *gc.C) {
	msg := "fail add-relation call at API"
	s.mockAPI.SetErrors(errors.New(msg))
	err := s.runAddRelation(c, "application1", "application2")
	c.Assert(err, gc.ErrorMatches, msg)
	s.mockAPI.CheckCallNames(c, "AddRelation", "Close")
}

func (s *AddRelationSuite) TestAddRelationBlocked(c *gc.C) {
	s.mockAPI.SetErrors(common.OperationBlockedError("TestBlockAddRelation"))
	err := s.runAddRelation(c, "application1", "application2")
	coretesting.AssertOperationWasBlocked(c, err, ".*TestBlockAddRelation.*")
	s.mockAPI.CheckCallNames(c, "AddRelation", "Close")
}

type mockAddAPI struct {
	*testing.Stub
	addRelationFunc func(endpoints ...string) (*params.AddRelationResults, error)
}

func (s mockAddAPI) Close() error {
	s.MethodCall(s, "Close")
	return s.NextErr()
}

func (s mockAddAPI) AddRelation(endpoints ...string) (*params.AddRelationResults, error) {
	s.MethodCall(s, "AddRelation", endpoints)
	return s.addRelationFunc(endpoints...)
}
