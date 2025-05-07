// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/tc"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cmd/juju/application"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
)

type SuspendRelationSuite struct {
	testhelpers.IsolationSuite
	mockAPI *mockSuspendAPI
}

func (s *SuspendRelationSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.mockAPI = &mockSuspendAPI{Stub: &testhelpers.Stub{}}
	s.mockAPI.setRelationSuspendedFunc = func(relationIds []int, suspended bool, message string) error {
		return s.mockAPI.NextErr()
	}
}

var _ = tc.Suite(&SuspendRelationSuite{})

func (s *SuspendRelationSuite) runSuspendRelation(c *tc.C, args ...string) error {
	store := jujuclienttesting.MinimalStore()
	_, err := cmdtesting.RunCommand(c, application.NewSuspendRelationCommandForTest(s.mockAPI, store), args...)
	return err
}

func (s *SuspendRelationSuite) TestSuspendRelationInvalidArguments(c *tc.C) {
	// No arguments
	err := s.runSuspendRelation(c)
	c.Assert(err, tc.ErrorMatches, "no relation ids specified")

	// argument not an integer
	err = s.runSuspendRelation(c, "application1")
	c.Assert(err, tc.ErrorMatches, `relation ID "application1" not valid`)
}

func (s *SuspendRelationSuite) TestSuspendRelationSuccess(c *tc.C) {
	err := s.runSuspendRelation(c, "123", "456", "--message", "message")
	c.Assert(err, tc.ErrorIsNil)
	s.mockAPI.CheckCall(c, 0, "SetRelationSuspended", []int{123, 456}, true, "message")
	s.mockAPI.CheckCall(c, 1, "Close")
}

func (s *SuspendRelationSuite) TestSuspendRelationFail(c *tc.C) {
	msg := "fail suspend-relation at API"
	s.mockAPI.SetErrors(errors.New(msg))
	err := s.runSuspendRelation(c, "123")
	c.Assert(err, tc.ErrorMatches, msg)
	s.mockAPI.CheckCall(c, 0, "SetRelationSuspended", []int{123}, true, "")
	s.mockAPI.CheckCall(c, 1, "Close")
}

func (s *SuspendRelationSuite) TestSuspendRelationBlocked(c *tc.C) {
	s.mockAPI.SetErrors(apiservererrors.OperationBlockedError("TestSuspendRelationBlocked"))
	err := s.runSuspendRelation(c, "123")
	coretesting.AssertOperationWasBlocked(c, err, ".*TestSuspendRelationBlocked.*")
	s.mockAPI.CheckCall(c, 0, "SetRelationSuspended", []int{123}, true, "")
	s.mockAPI.CheckCall(c, 1, "Close")
}

type mockSuspendAPI struct {
	*testhelpers.Stub
	setRelationSuspendedFunc func(relationIds []int, suspended bool, message string) error
}

func (s mockSuspendAPI) Close() error {
	s.MethodCall(s, "Close")
	return s.NextErr()
}

func (s mockSuspendAPI) SetRelationSuspended(ctx context.Context, relationIds []int, suspended bool, message string) error {
	s.MethodCall(s, "SetRelationSuspended", relationIds, suspended, message)
	return s.setRelationSuspendedFunc(relationIds, suspended, message)
}
