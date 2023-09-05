// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialcommon_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common/credentialcommon"
	"github.com/juju/juju/apiserver/common/credentialcommon/mocks"
	"github.com/juju/juju/rpc/params"
)

type CredentialSuite struct {
	testing.IsolationSuite

	backend *testBackend
}

var _ = gc.Suite(&CredentialSuite{})

func (s *CredentialSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.backend = newMockBackend()
}

func (s *CredentialSuite) TestInvalidateModelCredential(c *gc.C) {
	ctrl := gomock.NewController(c)
	credentialService := mocks.NewMockCredentialService(ctrl)
	api := credentialcommon.NewCredentialManagerAPI(s.backend, credentialService)

	credentialService.EXPECT().InvalidateCredential(gomock.Any(), s.backend.tag, "not again")

	result, err := api.InvalidateModelCredential(context.Background(), params.InvalidateCredentialArg{"not again"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResult{})
	s.backend.CheckCalls(c, []testing.StubCall{
		{"InvalidateModelCredential", []interface{}{"not again"}},
	})
}

func (s *CredentialSuite) TestInvalidateModelCredentialError(c *gc.C) {
	ctrl := gomock.NewController(c)
	credentialService := mocks.NewMockCredentialService(ctrl)
	api := credentialcommon.NewCredentialManagerAPI(s.backend, credentialService)

	expected := errors.New("boom")
	s.backend.SetErrors(expected)
	credentialService.EXPECT().InvalidateCredential(gomock.Any(), s.backend.tag, "not again")

	result, err := api.InvalidateModelCredential(context.Background(), params.InvalidateCredentialArg{"not again"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResult{Error: &params.Error{Message: "boom"}})
	s.backend.CheckCalls(c, []testing.StubCall{
		{"InvalidateModelCredential", []interface{}{"not again"}},
	})
}

func newMockBackend() *testBackend {
	return &testBackend{
		Stub: &testing.Stub{},
		tag:  names.NewCloudCredentialTag("cirrus/fred/default"),
	}
}

type testBackend struct {
	*testing.Stub
	tag names.CloudCredentialTag
}

func (b *testBackend) CloudCredentialTag() (names.CloudCredentialTag, bool) {
	return b.tag, true
}

func (b *testBackend) InvalidateModelCredential(reason string) error {
	b.AddCall("InvalidateModelCredential", reason)
	return b.NextErr()
}
