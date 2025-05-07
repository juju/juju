// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/testing"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/provider/common"
	"github.com/juju/juju/internal/provider/common/mocks"
)

type ErrorsSuite struct {
	testing.IsolationSuite

	credentialInvalidator *mocks.MockCredentialInvalidator
}

var _ = tc.Suite(&ErrorsSuite{})

func (*ErrorsSuite) TestWrapZoneIndependentError(c *tc.C) {
	err1 := errors.New("foo")
	err2 := errors.Annotate(err1, "bar")
	wrapped := environs.ZoneIndependentError(err2)
	c.Assert(wrapped, tc.ErrorIs, environs.ErrAvailabilityZoneIndependent)
	c.Assert(wrapped, tc.ErrorMatches, "bar: foo")
}

func (s *ErrorsSuite) TestInvalidCredentialWrapped(c *tc.C) {
	err1 := errors.New("foo")
	err2 := errors.Annotate(err1, "bar")
	err := common.CredentialNotValidError(err2)

	// This is to confirm that Is(err, ErrorCredentialNotValid) is correct.
	c.Assert(err, tc.ErrorIs, common.ErrorCredentialNotValid)
	c.Check(err, tc.ErrorMatches, "bar: foo")
}

func (s *ErrorsSuite) TestCredentialNotValidErrorLocationer(c *tc.C) {
	err := errors.New("some error")
	err = common.CredentialNotValidError(err)
	_, ok := err.(errors.Locationer)
	c.Assert(ok, tc.IsTrue)
}

func (s *ErrorsSuite) TestInvalidCredentialNew(c *tc.C) {
	err := fmt.Errorf("%w: Your account is blocked.", common.ErrorCredentialNotValid)
	c.Assert(err, tc.ErrorIs, common.ErrorCredentialNotValid)
	c.Check(err, tc.ErrorMatches, "credential not valid: Your account is blocked.")
}

func (s *ErrorsSuite) TestInvalidCredentialf(c *tc.C) {
	err1 := errors.New("foo")
	err := fmt.Errorf("bar: %w", common.CredentialNotValidError(err1))
	c.Assert(err, tc.ErrorIs, common.ErrorCredentialNotValid)
	c.Check(err, tc.ErrorMatches, "bar: foo")
}

var authFailureError = errors.New("auth failure")

func (s *ErrorsSuite) TestNoValidation(c *tc.C) {
	isAuthF := func(e error) bool {
		return true
	}
	denied, err := common.HandleCredentialError(context.Background(), nil, isAuthF, authFailureError)
	c.Assert(c.GetTestLog(), tc.Contains, "no credential invalidator provided")
	c.Assert(err, tc.Equals, authFailureError)
	c.Check(denied, tc.IsFalse)
}

func (s *ErrorsSuite) TestInvalidationCallbackErrorOnlyLogs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	isAuthF := func(e error) bool {
		return true
	}

	s.credentialInvalidator.EXPECT().InvalidateCredentials(gomock.Any(), gomock.Any()).Return(errors.New("boom"))

	denied, err := common.HandleCredentialError(context.Background(), s.credentialInvalidator, isAuthF, authFailureError)
	c.Assert(c.GetTestLog(), tc.Contains, "could not invalidate stored cloud credential on the controller")
	c.Assert(err, tc.Equals, authFailureError)
	c.Check(denied, tc.IsTrue)
}

func (s *ErrorsSuite) TestHandleCredentialErrorPermissionError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	isAuthF := func(e error) bool {
		return errors.Is(e, authFailureError)
	}

	var called bool
	s.credentialInvalidator.EXPECT().InvalidateCredentials(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, reason environs.CredentialInvalidReason) error {
		c.Assert(string(reason), tc.Matches, "cloud denied access:.*auth failure")
		called = true
		return nil
	})

	denied, err := common.HandleCredentialError(context.Background(), s.credentialInvalidator, isAuthF, authFailureError)
	c.Assert(called, tc.IsTrue)
	c.Assert(err, tc.Equals, authFailureError)
	c.Check(denied, tc.IsTrue)
}

func (s *ErrorsSuite) TestHandleCredentialErrorPermissionErrorTraced(c *tc.C) {
	defer s.setupMocks(c).Finish()

	isAuthF := func(e error) bool {
		return errors.Is(e, authFailureError)
	}

	var called bool
	s.credentialInvalidator.EXPECT().InvalidateCredentials(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, reason environs.CredentialInvalidReason) error {
		c.Assert(string(reason), tc.Matches, "cloud denied access:.*auth failure")
		called = true
		return nil
	})

	denied, err := common.HandleCredentialError(context.Background(), s.credentialInvalidator, isAuthF, errors.Trace(authFailureError))
	c.Assert(called, tc.IsTrue)
	c.Assert(err, tc.ErrorIs, authFailureError)
	c.Check(denied, tc.IsTrue)
}

func (s *ErrorsSuite) TestHandleCredentialErrorPermissionErrorAnnotated(c *tc.C) {
	defer s.setupMocks(c).Finish()

	isAuthF := func(e error) bool {
		return errors.Is(e, authFailureError)
	}

	var called bool
	s.credentialInvalidator.EXPECT().InvalidateCredentials(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, reason environs.CredentialInvalidReason) error {
		c.Assert(string(reason), tc.Matches, "cloud denied access:.*auth failure")
		called = true
		return nil
	})

	denied, err := common.HandleCredentialError(context.Background(), s.credentialInvalidator, isAuthF, errors.Annotatef(authFailureError, "annotated"))
	c.Assert(called, tc.IsTrue)
	c.Assert(err, tc.ErrorIs, authFailureError)
	c.Check(denied, tc.IsTrue)
}

func (s *ErrorsSuite) TestHandleCredentialErrorAnotherError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	isAuthF := func(e error) bool {
		return errors.Is(e, authFailureError)
	}

	denied, err := common.HandleCredentialError(context.Background(), s.credentialInvalidator, isAuthF, errors.New("some other error"))
	c.Assert(err, tc.ErrorMatches, "some other error")
	c.Assert(denied, tc.IsFalse)
}

func (s *ErrorsSuite) TestNilError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	isAuthF := func(e error) bool {
		return errors.Is(e, authFailureError)
	}

	denied, err := common.HandleCredentialError(context.Background(), s.credentialInvalidator, isAuthF, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(denied, tc.IsFalse)
}

func (s *ErrorsSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.credentialInvalidator = mocks.NewMockCredentialInvalidator(ctrl)
	return ctrl
}
