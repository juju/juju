// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facades/agent/credentialvalidator"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/domain/credential"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

type CredentialValidatorSuite struct {
	coretesting.BaseSuite

	resources         *common.Resources
	authorizer        apiservertesting.FakeAuthorizer
	backend           *testBackend
	cloudService      *testCloudService
	credentialService *testCredentialService

	api *credentialvalidator.CredentialValidatorAPI
}

var _ = gc.Suite(&CredentialValidatorSuite{})

func (s *CredentialValidatorSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.backend = newMockBackend()
	s.cloudService = newMockCloudService()
	s.credentialService = newMockCredentialService()

	s.resources = common.NewResources()
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("0"),
	}
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	api, err := credentialvalidator.NewCredentialValidatorAPIForTest(s.backend, s.cloudService, s.credentialService, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	s.api = api
}

func (s *CredentialValidatorSuite) TestModelCredential(c *gc.C) {
	result, err := s.api.ModelCredential(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ModelCredential{
		Model:           names.NewModelTag(modelUUID).String(),
		Exists:          true,
		CloudCredential: credentialTag.String(),
		Valid:           true,
	})
}

func (s *CredentialValidatorSuite) TestModelCredentialNotNeeded(c *gc.C) {
	s.backend.credentialSet = false
	result, err := s.api.ModelCredential(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ModelCredential{Model: names.NewModelTag(modelUUID).String()})
}

func (s *CredentialValidatorSuite) TestWatchCredential(c *gc.C) {
	result, err := s.api.WatchCredential(context.Background(), params.Entity{credentialTag.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResult{"1", nil})
	c.Assert(s.resources.Count(), gc.Equals, 1)
}

func (s *CredentialValidatorSuite) TestWatchCredentialNotUsedInThisModel(c *gc.C) {
	s.backend.credentialExists = false
	_, err := s.api.WatchCredential(context.Background(), params.Entity{"cloudcred-cloud_fred_default"})
	c.Assert(err, gc.ErrorMatches, apiservererrors.ErrPerm.Error())
	c.Assert(s.resources.Count(), gc.Equals, 0)
}

func (s *CredentialValidatorSuite) TestWatchCredentialInvalidTag(c *gc.C) {
	_, err := s.api.WatchCredential(context.Background(), params.Entity{"my-tag"})
	c.Assert(err, gc.ErrorMatches, `"my-tag" is not a valid tag`)
	c.Assert(s.resources.Count(), gc.Equals, 0)
}

func (s *CredentialValidatorSuite) TestInvalidateModelCredential(c *gc.C) {
	result, err := s.api.InvalidateModelCredential(context.Background(), params.InvalidateCredentialArg{"not again"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResult{})
	s.backend.CheckCalls(c, []testing.StubCall{
		{"mockModel.CloudCredentialTag", nil},
		{"InvalidateModelCredential", []interface{}{"not again"}},
	})
}

func (s *CredentialValidatorSuite) TestInvalidateModelCredentialError(c *gc.C) {
	expected := errors.New("boom")
	s.credentialService.SetErrors(expected)
	result, err := s.api.InvalidateModelCredential(context.Background(), params.InvalidateCredentialArg{"not again"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResult{Error: apiservererrors.ServerError(expected)})
	s.credentialService.CheckCalls(c, []testing.StubCall{
		{"InvalidateCredential", []interface{}{credential.IdFromTag(credentialTag), "not again"}},
	})
}

func (s *CredentialValidatorSuite) TestWatchModelCredential(c *gc.C) {
	result, err := s.api.WatchModelCredential(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResult{"1", nil})
	c.Assert(s.resources.Count(), gc.Equals, 1)
}

func (s *CredentialValidatorSuite) TestWatchModelCredentialError(c *gc.C) {
	s.backend.SetErrors(errors.New("no nope niet"))
	_, err := s.api.WatchModelCredential(context.Background())
	c.Assert(err, gc.ErrorMatches, "no nope niet")
	c.Assert(s.resources.Count(), gc.Equals, 0)
}
