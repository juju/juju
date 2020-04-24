// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialmanager_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/client/credentialmanager"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	coretesting "github.com/juju/juju/testing"
)

type CredentialManagerSuite struct {
	coretesting.BaseSuite

	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer
	backend    *testBackend

	api *credentialmanager.CredentialManagerAPI
}

var _ = gc.Suite(&CredentialManagerSuite{})

func (s *CredentialManagerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.backend = newMockBackend()

	s.resources = common.NewResources()
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:      names.NewUserTag("read"),
		AdminTag: names.NewUserTag("admin"),
	}
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	api, err := credentialmanager.NewCredentialManagerAPIForTest(s.backend, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	s.api = api
}

func (s *CredentialManagerSuite) TestInvalidateModelCredentialUnauthorized(c *gc.C) {
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("0"),
	}
	_, err := credentialmanager.NewCredentialManagerAPIForTest(s.backend, s.resources, s.authorizer)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *CredentialManagerSuite) TestInvalidateModelCredential(c *gc.C) {
	result, err := s.api.InvalidateModelCredential(params.InvalidateCredentialArg{"not again"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResult{})
	s.backend.CheckCalls(c, []testing.StubCall{
		{"InvalidateModelCredential", []interface{}{"not again"}},
	})
}

func (s *CredentialManagerSuite) TestInvalidateModelCredentialError(c *gc.C) {
	expected := errors.New("boom")
	s.backend.SetErrors(expected)
	result, err := s.api.InvalidateModelCredential(params.InvalidateCredentialArg{"not again"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResult{Error: common.ServerError(expected)})
	s.backend.CheckCalls(c, []testing.StubCall{
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

func (b *testBackend) InvalidateModelCredential(reason string) error {
	b.AddCall("InvalidateModelCredential", reason)
	return b.NextErr()
}
