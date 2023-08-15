// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialcommon_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common/credentialcommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/rpc/params"
)

type CredentialSuite struct {
	testing.IsolationSuite

	backend *testBackend
	api     *credentialcommon.CredentialManagerAPI
}

var _ = gc.Suite(&CredentialSuite{})

func (s *CredentialSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.backend = newMockBackend()
	s.api = credentialcommon.NewCredentialManagerAPI(s.backend)
}

func (s *CredentialSuite) TestInvalidateModelCredential(c *gc.C) {
	result, err := s.api.InvalidateModelCredential(context.Background(), params.InvalidateCredentialArg{"not again"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResult{})
	s.backend.CheckCalls(c, []testing.StubCall{
		{"InvalidateModelCredential", []interface{}{"not again"}},
	})
}

func (s *CredentialSuite) TestInvalidateModelCredentialError(c *gc.C) {
	expected := errors.New("boom")
	s.backend.SetErrors(expected)
	result, err := s.api.InvalidateModelCredential(context.Background(), params.InvalidateCredentialArg{"not again"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResult{Error: apiservererrors.ServerError(expected)})
	s.backend.CheckCalls(c, []testing.StubCall{
		{"InvalidateModelCredential", []interface{}{"not again"}},
	})
}

func newMockBackend() *testBackend {
	return &testBackend{Stub: &testing.Stub{}}
}

type testBackend struct {
	*testing.Stub
}

func (b *testBackend) InvalidateModelCredential(reason string) error {
	b.AddCall("InvalidateModelCredential", reason)
	return b.NextErr()
}
