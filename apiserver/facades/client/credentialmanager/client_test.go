// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialmanager_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/credentialcommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/client/credentialmanager"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

type CredentialManagerSuite struct {
	coretesting.BaseSuite

	resources         *common.Resources
	authorizer        apiservertesting.FakeAuthorizer
	backend           *testBackend
	credentialService *testCredentialService

	api *credentialmanager.CredentialManagerAPI
}

var _ = gc.Suite(&CredentialManagerSuite{})

func (s *CredentialManagerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.backend = newMockBackend()
	s.credentialService = newMockCredentialService()

	s.resources = common.NewResources()
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:      names.NewUserTag("read"),
		AdminTag: names.NewUserTag("admin"),
	}
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	api, err := credentialmanager.NewCredentialManagerAPIForTest(s.backend, s.credentialService, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	s.api = api
}

func (s *CredentialManagerSuite) TestInvalidateModelCredentialUnauthorized(c *gc.C) {
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("0"),
	}
	ctx := facadetest.ModelContext{
		Auth_: s.authorizer,
	}
	_, err := credentialmanager.NewCredentialManagerAPI(ctx)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *CredentialManagerSuite) TestInvalidateModelCredential(c *gc.C) {
	result, err := s.api.InvalidateModelCredential(context.Background(), params.InvalidateCredentialArg{"not again"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResult{})
	s.backend.CheckCalls(c, []testing.StubCall{
		{"CloudCredentialTag", []interface{}{}},
		{"InvalidateModelCredential", []interface{}{"not again"}},
	})
}

func (s *CredentialManagerSuite) TestInvalidateModelCredentialError(c *gc.C) {
	expected := errors.New("boom")
	s.backend.SetErrors(expected)
	result, err := s.api.InvalidateModelCredential(context.Background(), params.InvalidateCredentialArg{"not again"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResult{Error: apiservererrors.ServerError(expected)})
	s.backend.CheckCalls(c, []testing.StubCall{
		{"CloudCredentialTag", []interface{}{}},
		{"InvalidateModelCredential", []interface{}{"not again"}},
	})
}

func newMockBackend() *testBackend {
	return &testBackend{
		Stub: &testing.Stub{},
	}
}

type testBackend struct {
	*testing.Stub
}

func (b *testBackend) CloudCredentialTag() (names.CloudCredentialTag, bool, error) {
	b.AddCall("CloudCredentialTag")
	tag := names.NewCloudCredentialTag("cirrus/fred/default")
	return tag, true, nil
}

func newMockCredentialService() *testCredentialService {
	return &testCredentialService{
		Stub: &testing.Stub{},
	}
}

func (b *testBackend) InvalidateModelCredential(reason string) error {
	b.AddCall("InvalidateModelCredential", reason)
	return b.NextErr()
}

type testCredentialService struct {
	credentialcommon.CredentialService
	*testing.Stub
}

func (b *testCredentialService) InvalidateCredential(ctx context.Context, id credential.ID, reason string) error {
	b.AddCall("InvalidateCredential", id, reason)
	return b.NextErr()
}
