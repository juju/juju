// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	stderrors "errors"
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/txn"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/leadership"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type errorsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&errorsSuite{})

var errorTransformTests = []struct {
	err        error
	code       string
	status     int
	helperFunc func(error) bool
}{{
	err:        errors.NotFoundf("hello"),
	code:       params.CodeNotFound,
	status:     http.StatusNotFound,
	helperFunc: params.IsCodeNotFound,
}, {
	err:        errors.Unauthorizedf("hello"),
	code:       params.CodeUnauthorized,
	status:     http.StatusUnauthorized,
	helperFunc: params.IsCodeUnauthorized,
}, {
	err:        state.ErrCannotEnterScopeYet,
	code:       params.CodeCannotEnterScopeYet,
	status:     http.StatusInternalServerError,
	helperFunc: params.IsCodeCannotEnterScopeYet,
}, {
	err:        state.ErrCannotEnterScope,
	code:       params.CodeCannotEnterScope,
	status:     http.StatusInternalServerError,
	helperFunc: params.IsCodeCannotEnterScope,
}, {
	err:        state.ErrDead,
	code:       params.CodeDead,
	status:     http.StatusInternalServerError,
	helperFunc: params.IsCodeDead,
}, {
	err:        txn.ErrExcessiveContention,
	code:       params.CodeExcessiveContention,
	status:     http.StatusInternalServerError,
	helperFunc: params.IsCodeExcessiveContention,
}, {
	err:        state.ErrUnitHasSubordinates,
	code:       params.CodeUnitHasSubordinates,
	status:     http.StatusInternalServerError,
	helperFunc: params.IsCodeUnitHasSubordinates,
}, {
	err:        common.ErrBadId,
	code:       params.CodeNotFound,
	status:     http.StatusNotFound,
	helperFunc: params.IsCodeNotFound,
}, {
	err:        common.NoAddressSetError(names.NewUnitTag("mysql/0"), "public"),
	code:       params.CodeNoAddressSet,
	status:     http.StatusInternalServerError,
	helperFunc: params.IsCodeNoAddressSet,
}, {
	err:        common.ErrBadCreds,
	code:       params.CodeUnauthorized,
	status:     http.StatusUnauthorized,
	helperFunc: params.IsCodeUnauthorized,
}, {
	err:        common.ErrPerm,
	code:       params.CodeUnauthorized,
	status:     http.StatusUnauthorized,
	helperFunc: params.IsCodeUnauthorized,
}, {
	err:        common.ErrNotLoggedIn,
	code:       params.CodeUnauthorized,
	status:     http.StatusUnauthorized,
	helperFunc: params.IsCodeUnauthorized,
}, {
	err:        errors.NotProvisionedf("machine 0"),
	code:       params.CodeNotProvisioned,
	status:     http.StatusInternalServerError,
	helperFunc: params.IsCodeNotProvisioned,
}, {
	err:        errors.AlreadyExistsf("blah"),
	code:       params.CodeAlreadyExists,
	status:     http.StatusInternalServerError,
	helperFunc: params.IsCodeAlreadyExists,
}, {
	err:        common.ErrUnknownWatcher,
	code:       params.CodeNotFound,
	status:     http.StatusNotFound,
	helperFunc: params.IsCodeNotFound,
}, {
	err:        errors.NotAssignedf("unit mysql/0"),
	code:       params.CodeNotAssigned,
	status:     http.StatusInternalServerError,
	helperFunc: params.IsCodeNotAssigned,
}, {
	err:        common.ErrStoppedWatcher,
	code:       params.CodeStopped,
	status:     http.StatusInternalServerError,
	helperFunc: params.IsCodeStopped,
}, {
	err:        &state.HasAssignedUnitsError{"42", []string{"a"}},
	code:       params.CodeHasAssignedUnits,
	status:     http.StatusInternalServerError,
	helperFunc: params.IsCodeHasAssignedUnits,
}, {
	err:        common.ErrTryAgain,
	code:       params.CodeTryAgain,
	status:     http.StatusInternalServerError,
	helperFunc: params.IsCodeTryAgain,
}, {
	err:        state.UpgradeInProgressError,
	code:       params.CodeUpgradeInProgress,
	status:     http.StatusInternalServerError,
	helperFunc: params.IsCodeUpgradeInProgress,
}, {
	err:        leadership.ErrClaimDenied,
	code:       params.CodeLeadershipClaimDenied,
	status:     http.StatusInternalServerError,
	helperFunc: params.IsCodeLeadershipClaimDenied,
}, {
	err:        common.OperationBlockedError("test"),
	code:       params.CodeOperationBlocked,
	status:     http.StatusBadRequest,
	helperFunc: params.IsCodeOperationBlocked,
}, {
	err:        errors.NotSupportedf("needed feature"),
	code:       params.CodeNotSupported,
	status:     http.StatusInternalServerError,
	helperFunc: params.IsCodeNotSupported,
}, {
	err:        errors.BadRequestf("something"),
	code:       params.CodeBadRequest,
	status:     http.StatusBadRequest,
	helperFunc: params.IsBadRequest,
}, {
	err:        errors.MethodNotAllowedf("something"),
	code:       params.CodeMethodNotAllowed,
	status:     http.StatusMethodNotAllowed,
	helperFunc: params.IsMethodNotAllowed,
}, {
	err:    stderrors.New("an error"),
	status: http.StatusInternalServerError,
	code:   "",
}, {
	err: &common.DischargeRequiredError{
		Cause:    errors.New("something"),
		Macaroon: sampleMacaroon,
	},
	status: http.StatusUnauthorized,
	code:   params.CodeDischargeRequired,
	helperFunc: func(err error) bool {
		err1, ok := err.(*params.Error)
		if !ok || err1.Info == nil || err1.Info.Macaroon != sampleMacaroon {
			return false
		}
		return true
	},
}, {
	err:    unhashableError{"foo"},
	status: http.StatusInternalServerError,
	code:   "",
}, {
	err:        common.UnknownEnvironmentError("dead-beef-123456"),
	code:       params.CodeNotFound,
	status:     http.StatusNotFound,
	helperFunc: params.IsCodeNotFound,
}, {
	err:    nil,
	code:   "",
	status: http.StatusOK,
}}

var sampleMacaroon = func() *macaroon.Macaroon {
	m, err := macaroon.New([]byte("key"), "id", "loc")
	if err != nil {
		panic(err)
	}
	return m
}()

type unhashableError []string

func (err unhashableError) Error() string {
	return err[0]
}

func (s *errorsSuite) TestErrorTransform(c *gc.C) {
	for i, t := range errorTransformTests {
		c.Logf("test %d: %#v", i, t.err)
		err1, status := common.ServerErrorAndStatus(t.err)

		// Sanity check that ServerError returns the same thing.
		err2 := common.ServerError(t.err)
		c.Assert(err2, gc.DeepEquals, err1)
		c.Assert(status, gc.Equals, t.status)

		if t.err == nil {
			c.Assert(err1, gc.IsNil)
			c.Assert(status, gc.Equals, http.StatusOK)
			continue
		}
		c.Assert(err1.Message, gc.Equals, t.err.Error())
		c.Assert(err1.Code, gc.Equals, t.code)
		if t.helperFunc != nil {
			c.Assert(err1, jc.Satisfies, t.helperFunc)
		}
	}
}

func (s *errorsSuite) TestUnknownEnvironment(c *gc.C) {
	err := common.UnknownEnvironmentError("dead-beef")
	c.Check(err, gc.ErrorMatches, `unknown environment: "dead-beef"`)
}
