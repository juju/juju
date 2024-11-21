// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errorutils_test

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/internal/provider/azure/internal/errorutils"
	"github.com/juju/juju/internal/provider/common"
	"github.com/juju/juju/internal/testing"
)

type ErrorSuite struct {
	testing.BaseSuite

	azureError *azcore.ResponseError
}

var _ = gc.Suite(&ErrorSuite{})

func (s *ErrorSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.azureError = &azcore.ResponseError{
		StatusCode: http.StatusUnauthorized,
	}
}

func (s *ErrorSuite) TestNoValidation(c *gc.C) {
	ctx := envcontext.WithoutCredentialInvalidator(context.Background())
	err := errorutils.HandleCredentialError(s.azureError, ctx)
	c.Assert(err, gc.DeepEquals, s.azureError)

	denied := errorutils.MaybeInvalidateCredential(s.azureError, ctx)
	c.Assert(denied, jc.IsTrue)

	c.Assert(c.GetTestLog(), jc.DeepEquals, "")
}

func (s *ErrorSuite) TestHasDenialStatusCode(c *gc.C) {
	c.Assert(errorutils.HasDenialStatusCode(
		&azcore.ResponseError{StatusCode: http.StatusUnauthorized}), jc.IsTrue)
	c.Assert(errorutils.HasDenialStatusCode(
		&azcore.ResponseError{StatusCode: http.StatusNotFound}), jc.IsFalse)
	c.Assert(errorutils.HasDenialStatusCode(nil), jc.IsFalse)
	c.Assert(errorutils.HasDenialStatusCode(errors.New("FAIL")), jc.IsFalse)
}

func (s *ErrorSuite) TestInvalidationCallbackErrorOnlyLogs(c *gc.C) {
	ctx := envcontext.WithCredentialInvalidator(context.Background(), func(_ context.Context, msg string) error {
		return errors.New("kaboom")
	})
	errorutils.MaybeInvalidateCredential(s.azureError, ctx)
	c.Assert(c.GetTestLog(), jc.Contains, "could not invalidate stored azure cloud credential on the controller")
}

func (s *ErrorSuite) TestAuthRelatedStatusCodes(c *gc.C) {
	called := false
	ctx := envcontext.WithCredentialInvalidator(context.Background(), func(_ context.Context, msg string) error {
		c.Assert(msg, gc.Matches, "(?s)azure cloud denied access: .*")
		called = true
		return nil
	})

	// First test another status code.
	s.azureError.StatusCode = http.StatusAccepted
	errorutils.HandleCredentialError(s.azureError, ctx)
	c.Assert(called, jc.IsFalse)

	for t := range common.AuthorisationFailureStatusCodes {
		called = false
		s.azureError.StatusCode = t
		s.azureError.ErrorCode = "some error code"
		s.azureError.RawResponse = &http.Response{}
		errorutils.HandleCredentialError(s.azureError, ctx)
		c.Assert(called, jc.IsTrue)
	}
}

func (*ErrorSuite) TestNilAzureError(c *gc.C) {
	called := false
	ctx := envcontext.WithCredentialInvalidator(context.Background(), func(_ context.Context, msg string) error {
		called = true
		return nil
	})
	returnedErr := errorutils.HandleCredentialError(nil, ctx)
	c.Assert(called, jc.IsFalse)
	c.Assert(returnedErr, jc.ErrorIsNil)
}

func (*ErrorSuite) TestMaybeQuotaExceededError(c *gc.C) {
	buf := strings.NewReader(
		`{"error": {"code": "DeployError", "details": [{"code": "QuotaExceeded", "message": "boom"}]}}`)
	re := &azcore.ResponseError{
		StatusCode: http.StatusBadRequest,
		RawResponse: &http.Response{
			Body: io.NopCloser(buf),
		},
	}
	quotaErr, ok := errorutils.MaybeQuotaExceededError(re)
	c.Assert(ok, jc.IsTrue)
	c.Assert(quotaErr, gc.ErrorMatches, "boom")
}

func (*ErrorSuite) TestMaybeHypervisorGenNotSupportedError(c *gc.C) {
	buf := strings.NewReader(`
{"error":{"code":"DeployError","message":"","details":[{"code":"DeploymentFailed","message":"{\"error\":{\"code\":\"BadRequest\",\"message\":\"The selected VM size 'Standard_D2_v2' cannot boot Hypervisor Generation '2'. If this was a Create operation please check that the Hypervisor Generation of the Image matches the Hypervisor Generation of the selected VM Size. If this was an Update operation please select a Hypervisor Generation '2' VM Size. For more information, see https://aka.ms/azuregen2vm\",\"details\":null}}"}]}}`[1:])
	re := &azcore.ResponseError{
		StatusCode: http.StatusBadRequest,
		ErrorCode:  "DeploymentFailed",
		RawResponse: &http.Response{
			Body: io.NopCloser(buf),
		},
	}
	_, ok := errorutils.MaybeHypervisorGenNotSupportedError(re)
	c.Assert(ok, jc.IsTrue)
}

func (*ErrorSuite) TestIsConflictError(c *gc.C) {
	buf := strings.NewReader(
		`{"error": {"code": "DeployError", "details": [{"code": "Conflict", "message": "boom"}]}}`)

	re := &azcore.ResponseError{
		RawResponse: &http.Response{
			Body: io.NopCloser(buf),
		},
	}
	ok := errorutils.IsConflictError(re)
	c.Assert(ok, jc.IsTrue)

	se2 := &azcore.ResponseError{
		StatusCode: http.StatusConflict,
	}
	ok = errorutils.IsConflictError(se2)
	c.Assert(ok, jc.IsTrue)
}

func (*ErrorSuite) TestStatusCode(c *gc.C) {
	re := &azcore.ResponseError{
		StatusCode: http.StatusBadRequest,
	}
	code := errorutils.StatusCode(re)
	c.Assert(code, gc.Equals, http.StatusBadRequest)
}

func (*ErrorSuite) TestErrorCode(c *gc.C) {
	re := &azcore.ResponseError{
		ErrorCode: "failed",
	}
	code := errorutils.ErrorCode(re)
	c.Assert(code, gc.Equals, "failed")
}

func (*ErrorSuite) TestSimpleError(c *gc.C) {
	buf := strings.NewReader(
		`{"error": {"message": "failed"}}`)

	re := &azcore.ResponseError{
		RawResponse: &http.Response{
			Body: io.NopCloser(buf),
		},
	}

	err := errorutils.SimpleError(re)
	c.Assert(err, gc.ErrorMatches, "failed")
}
