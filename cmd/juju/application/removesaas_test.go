// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/juju/v2/api/client/application"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apiservererrors "github.com/juju/juju/v2/apiserver/errors"
	"github.com/juju/juju/v2/jujuclient/jujuclienttesting"
	"github.com/juju/juju/v2/rpc/params"
	coretesting "github.com/juju/juju/v2/testing"
)

type RemoveSaasSuite struct {
	testing.IsolationSuite

	mockAPI *mockRemoveSaasAPI
}

var _ = gc.Suite(&RemoveSaasSuite{})

func (s *RemoveSaasSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.mockAPI = &mockRemoveSaasAPI{Stub: &testing.Stub{}, version: 5}
}

func (s *RemoveSaasSuite) runRemoveSaas(c *gc.C, args ...string) (*cmd.Context, error) {
	store := jujuclienttesting.MinimalStore()
	return cmdtesting.RunCommand(c, NewRemoveSaasCommandForTest(s.mockAPI, store), args...)
}

func (s *RemoveSaasSuite) TestRemove(c *gc.C) {
	_, err := s.runRemoveSaas(c, "foo")
	c.Assert(err, jc.ErrorIsNil)
	destroyParams := application.DestroyConsumedApplicationParams{
		SaasNames: []string{"foo"},
		Force:     false,
		MaxWait:   (*time.Duration)(nil),
	}
	s.mockAPI.CheckCall(c, 0, "DestroyConsumedApplication", destroyParams)
	s.mockAPI.CheckCall(c, 1, "Close")
}

func (s *RemoveSaasSuite) TestBlockRemoveSaas(c *gc.C) {
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

func (s *RemoveSaasSuite) TestFailure(c *gc.C) {
	s.mockAPI.err = errors.New("an error")
	// Destroy an application that does not exist.
	ctx, err := s.runRemoveSaas(c, "gargleblaster")
	c.Assert(err, gc.Equals, cmd.ErrSilent)

	stderr := cmdtesting.Stderr(ctx)
	c.Assert(stderr, gc.Equals, `
removing SAAS application gargleblaster failed: an error
`[1:])
}

func (s *RemoveSaasSuite) TestInvalidArgs(c *gc.C) {
	_, err := s.runRemoveSaas(c)
	c.Assert(err, gc.ErrorMatches, `no SAAS application names specified`)
	_, err = s.runRemoveSaas(c, "invalid:name")
	c.Assert(err, gc.ErrorMatches, `invalid SAAS application name "invalid:name"`)
}

func (s *RemoveSaasSuite) TestRemoveSaasOldServer(c *gc.C) {
	s.mockAPI.version = 4
	_, err := s.runRemoveSaas(c, "foo")
	c.Assert(err, gc.ErrorMatches, "remove-saas is not supported by this version of Juju")
	s.mockAPI.CheckCall(c, 0, "Close")
}

type mockRemoveSaasAPI struct {
	*testing.Stub
	version int
	err     error
}

func (s mockRemoveSaasAPI) Close() error {
	s.MethodCall(s, "Close")
	return s.NextErr()
}

func (s mockRemoveSaasAPI) DestroyConsumedApplication(destroyParams application.DestroyConsumedApplicationParams) ([]params.ErrorResult, error) {
	s.MethodCall(s, "DestroyConsumedApplication", destroyParams)

	saasNames := destroyParams.SaasNames
	result := make([]params.ErrorResult, len(saasNames))
	for i := range saasNames {
		result[i].Error = apiservererrors.ServerError(s.err)
	}
	return result, s.NextErr()
}

func (s mockRemoveSaasAPI) BestAPIVersion() int {
	return s.version
}
