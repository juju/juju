// Copyright 2012 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"strings"

	"github.com/juju/cmd/v4/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cmd/juju/application"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/rpc/params"
	jtesting "github.com/juju/juju/testing"
)

type AddRelationSuite struct {
	testing.IsolationSuite
	mockAPI *mockAddAPI
}

func (s *AddRelationSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.mockAPI = &mockAddAPI{Stub: &testing.Stub{}}
	s.mockAPI.addRelationFunc = func(endpoints, viaCIDRs []string) (*params.AddRelationResults, error) {
		// At the moment, cmd implementation ignores the return values,
		// so nil is an acceptable return for testing purposes.
		return nil, s.mockAPI.NextErr()
	}
}

var _ = gc.Suite(&AddRelationSuite{})

func (s *AddRelationSuite) runAddRelation(c *gc.C, args ...string) error {
	cmd := application.NewAddRelationCommandForTest(s.mockAPI, s.mockAPI)
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	_, err := cmdtesting.RunCommand(c, cmd, args...)
	return err
}

func (s *AddRelationSuite) TestAddRelationWrongNumberOfArguments(c *gc.C) {
	// No arguments
	err := s.runAddRelation(c)
	c.Assert(err, gc.ErrorMatches, "an integration must involve two applications")

	// 1 argument
	err = s.runAddRelation(c, "application1")
	c.Assert(err, gc.ErrorMatches, "an integration must involve two applications")

	// more than 2 arguments
	err = s.runAddRelation(c, "application1", "application2", "application3")
	c.Assert(err, gc.ErrorMatches, "an integration must involve two applications")
}

func (s *AddRelationSuite) TestAddRelationSuccess(c *gc.C) {
	err := s.runAddRelation(c, "application1", "application2")
	c.Assert(err, jc.ErrorIsNil)
	s.mockAPI.CheckCall(c, 0, "AddRelation", []string{"application1", "application2"}, []string(nil))
	s.mockAPI.CheckCall(c, 1, "Close")
}

func (s *AddRelationSuite) TestAddRelationFail(c *gc.C) {
	msg := "fail integrate call at API"
	s.mockAPI.SetErrors(errors.New(msg))
	err := s.runAddRelation(c, "application1", "application2")
	c.Assert(err, gc.ErrorMatches, msg)
	s.mockAPI.CheckCall(c, 0, "AddRelation", []string{"application1", "application2"}, []string(nil))
	s.mockAPI.CheckCall(c, 1, "Close")
}

func (s *AddRelationSuite) TestAddRelationBlocked(c *gc.C) {
	s.mockAPI.SetErrors(apiservererrors.OperationBlockedError("TestBlockAddRelation"))
	err := s.runAddRelation(c, "application1", "application2")
	jtesting.AssertOperationWasBlocked(c, err, ".*TestBlockAddRelation.*")
	s.mockAPI.CheckCall(c, 0, "AddRelation", []string{"application1", "application2"}, []string(nil))
	s.mockAPI.CheckCall(c, 1, "Close")
}

func (s *AddRelationSuite) TestAddRelationUnauthorizedMentionsJujuGrant(c *gc.C) {
	s.mockAPI.SetErrors(&params.Error{
		Message: "permission denied",
		Code:    params.CodeUnauthorized,
	})
	cmd := application.NewAddRelationCommandForTest(s.mockAPI, s.mockAPI)
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	ctx, _ := cmdtesting.RunCommand(c, cmd, "application1", "application2")
	errString := strings.Replace(cmdtesting.Stderr(ctx), "\n", " ", -1)
	c.Assert(errString, gc.Matches, `.*juju grant.*`)
}

type mockAddAPI struct {
	*testing.Stub
	addRelationFunc func(endpoints, viaCIDRs []string) (*params.AddRelationResults, error)
}

func (s mockAddAPI) Close() error {
	s.MethodCall(s, "Close")
	return s.NextErr()
}

func (s mockAddAPI) AddRelation(endpoints, viaCIDRs []string) (*params.AddRelationResults, error) {
	s.MethodCall(s, "AddRelation", endpoints, viaCIDRs)
	return s.addRelationFunc(endpoints, viaCIDRs)
}

func (mockAddAPI) Consume(crossmodel.ConsumeApplicationArgs) (string, error) {
	return "", errors.New("unexpected method call: Consume")
}

func (mockAddAPI) GetConsumeDetails(string) (params.ConsumeOfferDetails, error) {
	return params.ConsumeOfferDetails{}, errors.New("unexpected method call: GetConsumeDetails")
}
