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
	s.mockAPI = &mockAddAPI{
		addRelationFunc: func(endpoints ...string) (*params.AddRelationResults, error) {
			// At the moment, cmd implementation ignores the return values,
			// so nil is an acceptable return for testing purposes.
			return nil, nil
		},
	}
}

var _ = gc.Suite(&AddRelationSuite{})

func (s *AddRelationSuite) runAddRelation(c *gc.C, args ...string) error {
	_, err := coretesting.RunCommand(c, NewAddRelationCommandForTest(s.mockAPI), args...)
	return err
}

func (s *AddRelationSuite) TestAddRelationNotEnoughArguments(c *gc.C) {
	// No arguments
	err := s.runAddRelation(c)
	c.Assert(err, gc.ErrorMatches, "a relation must involve two applications")

	// 1 empty argument
	err = s.runAddRelation(c, "")
	c.Assert(err, gc.ErrorMatches, "a relation must involve two applications")

	// 2 empty arguments
	err = s.runAddRelation(c, "", "")
	c.Assert(err, gc.ErrorMatches, "a relation must involve two applications")

	// 1 empty and 1 non-empty arguments
	err = s.runAddRelation(c, "application1", "")
	c.Assert(err, gc.ErrorMatches, "a relation must involve two applications")

	// 1 empty and 1 non-empty arguments
	err = s.runAddRelation(c, "", "application2")
	c.Assert(err, gc.ErrorMatches, "a relation must involve two applications")
}

func (s *AddRelationSuite) TestAddRelationSuccess(c *gc.C) {
	err := s.runAddRelation(c, "application1", "application2")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *AddRelationSuite) TestAddRelationFail(c *gc.C) {
	msg := "fail add relation"
	s.mockAPI = &mockAddAPI{
		addRelationFunc: func(endpoints ...string) (*params.AddRelationResults, error) {
			return nil, errors.New(msg)
		},
	}
	err := s.runAddRelation(c, "application1", "application2")
	c.Assert(err, gc.ErrorMatches, msg)
}

func (s *AddRelationSuite) TestAddRelationBlocked(c *gc.C) {
	// Block operation
	s.mockAPI.addRelationFunc = func(endpoints ...string) (*params.AddRelationResults, error) {
		return nil, common.OperationBlockedError("TestBlockAddRelation")
	}
	err := s.runAddRelation(c, "application1", "application2")
	coretesting.AssertOperationWasBlocked(c, err, ".*TestBlockAddRelation.*")
}

type mockAddAPI struct {
	addRelationFunc func(endpoints ...string) (*params.AddRelationResults, error)
}

func (s mockAddAPI) Close() error {
	return nil
}

func (s mockAddAPI) AddRelation(endpoints ...string) (*params.AddRelationResults, error) {
	return s.addRelationFunc(endpoints...)
}
