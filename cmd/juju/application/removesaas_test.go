// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"
	stdtesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/api/client/application"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/rpc/params"
)

type RemoveSaasSuite struct {
	testhelpers.IsolationSuite

	mockAPI *mockRemoveSaasAPI
}

func TestRemoveSaasSuite(t *stdtesting.T) {
	tc.Run(t, &RemoveSaasSuite{})
}

func (s *RemoveSaasSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.mockAPI = &mockRemoveSaasAPI{Stub: &testhelpers.Stub{}}
}

func (s *RemoveSaasSuite) runRemoveSaas(c *tc.C, args ...string) (*cmd.Context, error) {
	store := jujuclienttesting.MinimalStore()
	return cmdtesting.RunCommand(c, NewRemoveSaasCommandForTest(s.mockAPI, store), args...)
}

func (s *RemoveSaasSuite) TestRemove(c *tc.C) {
	_, err := s.runRemoveSaas(c, "foo")
	c.Assert(err, tc.ErrorIsNil)
	destroyParams := application.DestroyConsumedApplicationParams{
		SaasNames: []string{"foo"},
		Force:     false,
		MaxWait:   (*time.Duration)(nil),
	}
	s.mockAPI.CheckCall(c, 0, "DestroyConsumedApplication", destroyParams)
	s.mockAPI.CheckCall(c, 1, "Close")
}

func (s *RemoveSaasSuite) TestBlockRemoveSaas(c *tc.C) {
	s.mockAPI.SetErrors(apiservererrors.OperationBlockedError("TestRemoveSaasBlocked"))
	_, err := s.runRemoveSaas(c, "foo")
	coretesting.AssertOperationWasBlocked(c, err, ".*TestRemoveSaasBlocked.*")
	destroyParams := application.DestroyConsumedApplicationParams{
		SaasNames: []string{"foo"},
		Force:     false,
		MaxWait:   (*time.Duration)(nil),
	}
	s.mockAPI.CheckCall(c, 0, "DestroyConsumedApplication", destroyParams)
	s.mockAPI.CheckCall(c, 1, "Close")
}

func (s *RemoveSaasSuite) TestFailure(c *tc.C) {
	s.mockAPI.err = errors.New("an error")
	// Destroy an application that does not exist.
	ctx, err := s.runRemoveSaas(c, "gargleblaster")
	c.Assert(err, tc.Equals, cmd.ErrSilent)

	stderr := cmdtesting.Stderr(ctx)
	c.Assert(stderr, tc.Equals, `
removing SAAS application gargleblaster failed: an error
`[1:])
}

func (s *RemoveSaasSuite) TestInvalidArgs(c *tc.C) {
	_, err := s.runRemoveSaas(c)
	c.Assert(err, tc.ErrorMatches, `no SAAS application names specified`)
	_, err = s.runRemoveSaas(c, "invalid:name")
	c.Assert(err, tc.ErrorMatches, `invalid SAAS application name "invalid:name"`)
}

type mockRemoveSaasAPI struct {
	*testhelpers.Stub
	err error
}

func (s mockRemoveSaasAPI) Close() error {
	s.MethodCall(s, "Close")
	return s.NextErr()
}

func (s mockRemoveSaasAPI) DestroyConsumedApplication(ctx context.Context, destroyParams application.DestroyConsumedApplicationParams) ([]params.ErrorResult, error) {
	s.MethodCall(s, "DestroyConsumedApplication", destroyParams)

	saasNames := destroyParams.SaasNames
	result := make([]params.ErrorResult, len(saasNames))
	for i := range saasNames {
		result[i].Error = apiservererrors.ServerError(s.err)
	}
	return result, s.NextErr()
}
