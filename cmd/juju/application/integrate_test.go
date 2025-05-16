// Copyright 2012 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"context"
	"strings"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cmd/juju/application"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testhelpers"
	jtesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/rpc/params"
)

type AddRelationSuite struct {
	testhelpers.IsolationSuite
	mockAPI *mockAddAPI
}

func (s *AddRelationSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.mockAPI = &mockAddAPI{Stub: &testhelpers.Stub{}}
	s.mockAPI.addRelationFunc = func(endpoints, viaCIDRs []string) (*params.AddRelationResults, error) {
		// At the moment, cmd implementation ignores the return values,
		// so nil is an acceptable return for testing purposes.
		return nil, s.mockAPI.NextErr()
	}
}
func TestAddRelationSuite(t *stdtesting.T) { tc.Run(t, &AddRelationSuite{}) }
func (s *AddRelationSuite) runAddRelation(c *tc.C, args ...string) error {
	cmd := application.NewAddRelationCommandForTest(s.mockAPI, s.mockAPI)
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	_, err := cmdtesting.RunCommand(c, cmd, args...)
	return err
}

func (s *AddRelationSuite) TestAddRelationWrongNumberOfArguments(c *tc.C) {
	// No arguments
	err := s.runAddRelation(c)
	c.Assert(err, tc.ErrorMatches, "an integration must involve two applications")

	// 1 argument
	err = s.runAddRelation(c, "application1")
	c.Assert(err, tc.ErrorMatches, "an integration must involve two applications")

	// more than 2 arguments
	err = s.runAddRelation(c, "application1", "application2", "application3")
	c.Assert(err, tc.ErrorMatches, "an integration must involve two applications")
}

func (s *AddRelationSuite) TestAddRelationSuccess(c *tc.C) {
	err := s.runAddRelation(c, "application1", "application2")
	c.Assert(err, tc.ErrorIsNil)
	s.mockAPI.CheckCall(c, 0, "AddRelation", []string{"application1", "application2"}, []string(nil))
	s.mockAPI.CheckCall(c, 1, "Close")
}

func (s *AddRelationSuite) TestAddRelationFail(c *tc.C) {
	msg := "fail integrate call at API"
	s.mockAPI.SetErrors(errors.New(msg))
	err := s.runAddRelation(c, "application1", "application2")
	c.Assert(err, tc.ErrorMatches, msg)
	s.mockAPI.CheckCall(c, 0, "AddRelation", []string{"application1", "application2"}, []string(nil))
	s.mockAPI.CheckCall(c, 1, "Close")
}

func (s *AddRelationSuite) TestAddRelationBlocked(c *tc.C) {
	s.mockAPI.SetErrors(apiservererrors.OperationBlockedError("TestBlockAddRelation"))
	err := s.runAddRelation(c, "application1", "application2")
	jtesting.AssertOperationWasBlocked(c, err, ".*TestBlockAddRelation.*")
	s.mockAPI.CheckCall(c, 0, "AddRelation", []string{"application1", "application2"}, []string(nil))
	s.mockAPI.CheckCall(c, 1, "Close")
}

func (s *AddRelationSuite) TestAddRelationUnauthorizedMentionsJujuGrant(c *tc.C) {
	s.mockAPI.SetErrors(&params.Error{
		Message: "permission denied",
		Code:    params.CodeUnauthorized,
	})
	cmd := application.NewAddRelationCommandForTest(s.mockAPI, s.mockAPI)
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	ctx, _ := cmdtesting.RunCommand(c, cmd, "application1", "application2")
	errString := strings.Replace(cmdtesting.Stderr(ctx), "\n", " ", -1)
	c.Assert(errString, tc.Matches, `.*juju grant.*`)
}

type mockAddAPI struct {
	*testhelpers.Stub
	addRelationFunc func(endpoints, viaCIDRs []string) (*params.AddRelationResults, error)
}

func (s mockAddAPI) Close() error {
	s.MethodCall(s, "Close")
	return s.NextErr()
}

func (s mockAddAPI) AddRelation(ctx context.Context, endpoints, viaCIDRs []string) (*params.AddRelationResults, error) {
	s.MethodCall(s, "AddRelation", endpoints, viaCIDRs)
	return s.addRelationFunc(endpoints, viaCIDRs)
}

func (mockAddAPI) Consume(context.Context, crossmodel.ConsumeApplicationArgs) (string, error) {
	return "", errors.New("unexpected method call: Consume")
}

func (mockAddAPI) GetConsumeDetails(context.Context, string) (params.ConsumeOfferDetails, error) {
	return params.ConsumeOfferDetails{}, errors.New("unexpected method call: GetConsumeDetails")
}
