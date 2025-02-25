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
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	environs "github.com/juju/juju/environs"
	"github.com/juju/juju/internal/provider/azure/internal/errorutils"
	"github.com/juju/juju/internal/provider/common"
	"github.com/juju/juju/internal/testing"
)

type ErrorSuite struct {
	testing.BaseSuite

	invalidator *MockCredentialInvalidator

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
	defer s.setupMocks(c).Finish()

	handled, err := errorutils.HandleCredentialError(context.Background(), nil, s.azureError)
	c.Assert(err, jc.ErrorIs, s.azureError)
	c.Check(handled, jc.IsFalse)
	c.Check(c.GetTestLog(), jc.Contains, "no credential invalidator provided to handle error")
}

func (s *ErrorSuite) TestHasDenialStatusCode(c *gc.C) {
	defer s.setupMocks(c).Finish()

	c.Assert(errorutils.HasDenialStatusCode(
		&azcore.ResponseError{StatusCode: http.StatusUnauthorized}), jc.IsTrue)
	c.Assert(errorutils.HasDenialStatusCode(
		&azcore.ResponseError{StatusCode: http.StatusNotFound}), jc.IsFalse)
	c.Assert(errorutils.HasDenialStatusCode(nil), jc.IsFalse)
	c.Assert(errorutils.HasDenialStatusCode(errors.New("FAIL")), jc.IsFalse)
}

func (s *ErrorSuite) TestInvalidationCallbackErrorOnlyLogs(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.invalidator.EXPECT().InvalidateCredentials(gomock.Any(), gomock.Any()).Return(errors.New("kaboom"))

	handled, err := errorutils.HandleCredentialError(context.Background(), s.invalidator, s.azureError)
	c.Assert(err, jc.ErrorIs, s.azureError)
	c.Check(handled, jc.IsTrue)
	c.Check(c.GetTestLog(), jc.Contains, "could not invalidate stored cloud credential on the controller")
}

func (s *ErrorSuite) TestAuthRelatedStatusCodes(c *gc.C) {
	defer s.setupMocks(c).Finish()

	var called bool
	s.invalidator.EXPECT().InvalidateCredentials(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, reason environs.CredentialInvalidReason) error {
		c.Assert(string(reason), jc.Contains, "azure cloud denied access")
		called = true
		return nil
	}).Times(common.AuthorisationFailureStatusCodes.Size())

	// First test another status code.
	s.azureError.StatusCode = http.StatusAccepted
	handled, err := errorutils.HandleCredentialError(context.Background(), s.invalidator, s.azureError)
	c.Assert(err, jc.ErrorIs, s.azureError)
	c.Check(handled, jc.IsFalse)
	c.Check(called, jc.IsFalse)

	for t := range common.AuthorisationFailureStatusCodes {
		called = false

		s.azureError.StatusCode = t
		s.azureError.ErrorCode = "some error code"
		s.azureError.RawResponse = &http.Response{}

		handled, err := errorutils.HandleCredentialError(context.Background(), s.invalidator, s.azureError)
		c.Assert(err, jc.ErrorIs, s.azureError)
		c.Check(handled, jc.IsTrue)
		c.Check(called, jc.IsTrue)
	}
}

func (s *ErrorSuite) TestNilAzureError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	handled, returnedErr := errorutils.HandleCredentialError(context.Background(), s.invalidator, nil)
	c.Assert(returnedErr, jc.ErrorIsNil)
	c.Assert(handled, jc.IsFalse)
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

func (s *ErrorSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.invalidator = NewMockCredentialInvalidator(ctrl)

	return ctrl
}
