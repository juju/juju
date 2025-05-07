// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
)

type ResumeRelationSuite struct {
	testing.IsolationSuite
	mockAPI *mockResumeAPI
}

func (s *ResumeRelationSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.mockAPI = &mockResumeAPI{Stub: &testing.Stub{}}
	s.mockAPI.setRelationSuspendedFunc = func(relationIds []int, suspended bool, message string) error {
		return s.mockAPI.NextErr()
	}
}

var _ = tc.Suite(&ResumeRelationSuite{})

func (s *ResumeRelationSuite) runResumeRelation(c *tc.C, args ...string) error {
	store := jujuclienttesting.MinimalStore()
	_, err := cmdtesting.RunCommand(c, NewResumeRelationCommandForTest(s.mockAPI, store), args...)
	return err
}

func (s *ResumeRelationSuite) TestResumeRelationInvalidArguments(c *tc.C) {
	// No arguments
	err := s.runResumeRelation(c)
	c.Assert(err, tc.ErrorMatches, "no relation ids specified")

	// argument not an integer
	err = s.runResumeRelation(c, "application1")
	c.Assert(err, tc.ErrorMatches, `relation ID "application1" not valid`)
}

func (s *ResumeRelationSuite) TestResumeRelationSuccess(c *tc.C) {
	err := s.runResumeRelation(c, "123")
	c.Assert(err, jc.ErrorIsNil)
	s.mockAPI.CheckCall(c, 0, "SetRelationSuspended", []int{123}, false, "")
	s.mockAPI.CheckCall(c, 1, "Close")
}

func (s *ResumeRelationSuite) TestResumeRelationFail(c *tc.C) {
	msg := "fail resume-relation at API"
	s.mockAPI.SetErrors(errors.New(msg))
	err := s.runResumeRelation(c, "123", "456")
	c.Assert(err, tc.ErrorMatches, msg)
	s.mockAPI.CheckCall(c, 0, "SetRelationSuspended", []int{123, 456}, false, "")
	s.mockAPI.CheckCall(c, 1, "Close")
}

func (s *ResumeRelationSuite) TestResumeRelationBlocked(c *tc.C) {
	s.mockAPI.SetErrors(apiservererrors.OperationBlockedError("TestResumeRelationBlocked"))
	err := s.runResumeRelation(c, "123")
	coretesting.AssertOperationWasBlocked(c, err, ".*TestResumeRelationBlocked.*")
	s.mockAPI.CheckCall(c, 0, "SetRelationSuspended", []int{123}, false, "")
	s.mockAPI.CheckCall(c, 1, "Close")
}

type mockResumeAPI struct {
	*testing.Stub
	setRelationSuspendedFunc func(relationIds []int, suspended bool, message string) error
}

func (s mockResumeAPI) Close() error {
	s.MethodCall(s, "Close")
	return s.NextErr()
}

func (s mockResumeAPI) SetRelationSuspended(ctx context.Context, relationIds []int, suspended bool, message string) error {
	s.MethodCall(s, "SetRelationSuspended", relationIds, suspended, message)
	return s.setRelationSuspendedFunc(relationIds, suspended, message)
}
