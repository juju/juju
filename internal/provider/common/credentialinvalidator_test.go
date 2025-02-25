// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/provider/common"
	"github.com/juju/juju/internal/provider/common/mocks"
)

type ErrorsSuite struct {
	testing.IsolationSuite

	credentialInvalidator *mocks.MockCredentialInvalidator
}

var _ = gc.Suite(&ErrorsSuite{})

func (*ErrorsSuite) TestWrapZoneIndependentError(c *gc.C) {
	err1 := errors.New("foo")
	err2 := errors.Annotate(err1, "bar")
	wrapped := environs.ZoneIndependentError(err2)
	c.Assert(wrapped, jc.ErrorIs, environs.ErrAvailabilityZoneIndependent)
	c.Assert(wrapped, gc.ErrorMatches, "bar: foo")
}

func (s *ErrorsSuite) TestInvalidCredentialWrapped(c *gc.C) {
	err1 := errors.New("foo")
	err2 := errors.Annotate(err1, "bar")
	err := common.CredentialNotValidError(err2)

	// This is to confirm that Is(err, ErrorCredentialNotValid) is correct.
	c.Assert(err, jc.ErrorIs, common.ErrorCredentialNotValid)
	c.Check(err, gc.ErrorMatches, "bar: foo")
}

func (s *ErrorsSuite) TestCredentialNotValidErrorLocationer(c *gc.C) {
	err := errors.New("some error")
	err = common.CredentialNotValidError(err)
	_, ok := err.(errors.Locationer)
	c.Assert(ok, jc.IsTrue)
}

func (s *ErrorsSuite) TestInvalidCredentialNew(c *gc.C) {
	err := fmt.Errorf("%w: Your account is blocked.", common.ErrorCredentialNotValid)
	c.Assert(err, jc.ErrorIs, common.ErrorCredentialNotValid)
	c.Check(err, gc.ErrorMatches, "credential not valid: Your account is blocked.")
}

func (s *ErrorsSuite) TestInvalidCredentialf(c *gc.C) {
	err1 := errors.New("foo")
	err := fmt.Errorf("bar: %w", common.CredentialNotValidError(err1))
	c.Assert(err, jc.ErrorIs, common.ErrorCredentialNotValid)
	c.Check(err, gc.ErrorMatches, "bar: foo")
}

var authFailureError = errors.New("auth failure")

func (s *ErrorsSuite) TestNoValidation(c *gc.C) {
	isAuthF := func(e error) bool {
		return true
	}
	denied, err := common.HandleCredentialError(context.Background(), nil, isAuthF, authFailureError)
	c.Assert(c.GetTestLog(), jc.Contains, "no credential invalidator provided")
	c.Assert(err, gc.Equals, authFailureError)
	c.Check(denied, jc.IsFalse)
}

func (s *ErrorsSuite) TestInvalidationCallbackErrorOnlyLogs(c *gc.C) {
	defer s.setupMocks(c).Finish()

	isAuthF := func(e error) bool {
		return true
	}

	s.credentialInvalidator.EXPECT().InvalidateCredentials(gomock.Any(), gomock.Any()).Return(errors.New("boom"))

	denied, err := common.HandleCredentialError(context.Background(), s.credentialInvalidator, isAuthF, authFailureError)
	c.Assert(c.GetTestLog(), jc.Contains, "could not invalidate stored cloud credential on the controller")
	c.Assert(err, gc.Equals, authFailureError)
	c.Check(denied, jc.IsTrue)
}

func (s *ErrorsSuite) TestHandleCredentialErrorPermissionError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	isAuthF := func(e error) bool {
		return errors.Is(e, authFailureError)
	}

	var called bool
	s.credentialInvalidator.EXPECT().InvalidateCredentials(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, reason environs.CredentialInvalidReason) error {
		c.Assert(string(reason), gc.Matches, "cloud denied access:.*auth failure")
		called = true
		return nil
	})

	denied, err := common.HandleCredentialError(context.Background(), s.credentialInvalidator, isAuthF, authFailureError)
	c.Assert(called, jc.IsTrue)
	c.Assert(err, gc.Equals, authFailureError)
	c.Check(denied, jc.IsTrue)
}

func (s *ErrorsSuite) TestHandleCredentialErrorPermissionErrorTraced(c *gc.C) {
	defer s.setupMocks(c).Finish()

	isAuthF := func(e error) bool {
		return errors.Is(e, authFailureError)
	}

	var called bool
	s.credentialInvalidator.EXPECT().InvalidateCredentials(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, reason environs.CredentialInvalidReason) error {
		c.Assert(string(reason), gc.Matches, "cloud denied access:.*auth failure")
		called = true
		return nil
	})

	denied, err := common.HandleCredentialError(context.Background(), s.credentialInvalidator, isAuthF, errors.Trace(authFailureError))
	c.Assert(called, jc.IsTrue)
	c.Assert(err, jc.ErrorIs, authFailureError)
	c.Check(denied, jc.IsTrue)
}

func (s *ErrorsSuite) TestHandleCredentialErrorPermissionErrorAnnotated(c *gc.C) {
	defer s.setupMocks(c).Finish()

	isAuthF := func(e error) bool {
		return errors.Is(e, authFailureError)
	}

	var called bool
	s.credentialInvalidator.EXPECT().InvalidateCredentials(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, reason environs.CredentialInvalidReason) error {
		c.Assert(string(reason), gc.Matches, "cloud denied access:.*auth failure")
		called = true
		return nil
	})

	denied, err := common.HandleCredentialError(context.Background(), s.credentialInvalidator, isAuthF, errors.Annotatef(authFailureError, "annotated"))
	c.Assert(called, jc.IsTrue)
	c.Assert(err, jc.ErrorIs, authFailureError)
	c.Check(denied, jc.IsTrue)
}

func (s *ErrorsSuite) TestHandleCredentialErrorAnotherError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	isAuthF := func(e error) bool {
		return errors.Is(e, authFailureError)
	}

	denied, err := common.HandleCredentialError(context.Background(), s.credentialInvalidator, isAuthF, errors.New("some other error"))
	c.Assert(err, gc.ErrorMatches, "some other error")
	c.Assert(denied, jc.IsFalse)
}

func (s *ErrorsSuite) TestNilError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	isAuthF := func(e error) bool {
		return errors.Is(e, authFailureError)
	}

	denied, err := common.HandleCredentialError(context.Background(), s.credentialInvalidator, isAuthF, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(denied, jc.IsFalse)
}

func (s *ErrorsSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.credentialInvalidator = mocks.NewMockCredentialInvalidator(ctrl)
	return ctrl
}
