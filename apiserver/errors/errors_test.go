// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors_test

import (
	"encoding/json"
	stderrors "errors"
	"net/http"
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/txn/v2"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/network"
	stateerrors "github.com/juju/juju/state/errors"
	"github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
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
	err:        errors.UserNotFoundf("xxxx"),
	code:       params.CodeUserNotFound,
	status:     http.StatusNotFound,
	helperFunc: params.IsCodeUserNotFound,
}, {
	err:        errors.Unauthorizedf("hello"),
	code:       params.CodeUnauthorized,
	status:     http.StatusUnauthorized,
	helperFunc: params.IsCodeUnauthorized,
}, {
	err:        stateerrors.ErrCannotEnterScopeYet,
	code:       params.CodeCannotEnterScopeYet,
	status:     http.StatusInternalServerError,
	helperFunc: params.IsCodeCannotEnterScopeYet,
}, {
	err:        stateerrors.ErrCannotEnterScope,
	code:       params.CodeCannotEnterScope,
	status:     http.StatusInternalServerError,
	helperFunc: params.IsCodeCannotEnterScope,
}, {
	err:        stateerrors.ErrDead,
	code:       params.CodeDead,
	status:     http.StatusInternalServerError,
	helperFunc: params.IsCodeDead,
}, {
	err:        txn.ErrExcessiveContention,
	code:       params.CodeExcessiveContention,
	status:     http.StatusInternalServerError,
	helperFunc: params.IsCodeExcessiveContention,
}, {
	err:        stateerrors.ErrUnitHasSubordinates,
	code:       params.CodeUnitHasSubordinates,
	status:     http.StatusInternalServerError,
	helperFunc: params.IsCodeUnitHasSubordinates,
}, {
	err:        apiservererrors.ErrBadId,
	code:       params.CodeNotFound,
	status:     http.StatusNotFound,
	helperFunc: params.IsCodeNotFound,
}, {
	err:        apiservererrors.NoAddressSetError(names.NewUnitTag("mysql/0"), "public"),
	code:       params.CodeNoAddressSet,
	status:     http.StatusInternalServerError,
	helperFunc: params.IsCodeNoAddressSet,
}, {
	err:        apiservererrors.ErrBadCreds,
	code:       params.CodeUnauthorized,
	status:     http.StatusUnauthorized,
	helperFunc: params.IsCodeUnauthorized,
}, {
	err:        apiservererrors.ErrPerm,
	code:       params.CodeUnauthorized,
	status:     http.StatusUnauthorized,
	helperFunc: params.IsCodeUnauthorized,
}, {
	err:        apiservererrors.ErrNotLoggedIn,
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
	err:        apiservererrors.ErrUnknownWatcher,
	code:       params.CodeNotFound,
	status:     http.StatusNotFound,
	helperFunc: params.IsCodeNotFound,
}, {
	err:        errors.NotAssignedf("unit mysql/0"),
	code:       params.CodeNotAssigned,
	status:     http.StatusInternalServerError,
	helperFunc: params.IsCodeNotAssigned,
}, {
	err:        apiservererrors.ErrStoppedWatcher,
	code:       params.CodeStopped,
	status:     http.StatusInternalServerError,
	helperFunc: params.IsCodeStopped,
}, {
	err:        stateerrors.NewHasAssignedUnitsError("42", []string{"a"}),
	code:       params.CodeHasAssignedUnits,
	status:     http.StatusInternalServerError,
	helperFunc: params.IsCodeHasAssignedUnits,
}, {
	err:        apiservererrors.ErrTryAgain,
	code:       params.CodeTryAgain,
	status:     http.StatusInternalServerError,
	helperFunc: params.IsCodeTryAgain,
}, {
	err:        leadership.ErrClaimDenied,
	code:       params.CodeLeadershipClaimDenied,
	status:     http.StatusInternalServerError,
	helperFunc: params.IsCodeLeadershipClaimDenied,
}, {
	err:        lease.ErrClaimDenied,
	code:       params.CodeLeaseClaimDenied,
	status:     http.StatusInternalServerError,
	helperFunc: params.IsCodeLeaseClaimDenied,
}, {
	err:        apiservererrors.OperationBlockedError("test"),
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
	err: &apiservererrors.DischargeRequiredError{
		Cause:          errors.New("something"),
		LegacyMacaroon: sampleMacaroon,
	},
	status: http.StatusUnauthorized,
	code:   params.CodeDischargeRequired,
	helperFunc: func(err error) bool {
		err1, ok := err.(*params.Error)
		exp := asMap(sampleMacaroon)
		if !ok || err1.Info == nil || !reflect.DeepEqual(err1.Info["macaroon"], exp) {
			return false
		}
		return true
	},
}, {
	err:    unhashableError{"foo"},
	status: http.StatusInternalServerError,
	code:   "",
}, {
	err:        apiservererrors.UnknownModelError("dead-beef-123456"),
	code:       params.CodeModelNotFound,
	status:     http.StatusNotFound,
	helperFunc: params.IsCodeModelNotFound,
}, {
	err:    sampleRedirectError,
	status: http.StatusMovedPermanently,
	code:   params.CodeRedirect,
	helperFunc: func(err error) bool {
		err1, ok := err.(*params.Error)
		exp := asMap(params.RedirectErrorInfo{
			Servers: params.FromProviderHostsPorts(sampleRedirectError.Servers),
			CACert:  sampleRedirectError.CACert,
		})
		if !ok || err1.Info == nil || !reflect.DeepEqual(err1.Info, exp) {
			return false
		}
		return true
	},
}, {
	err:        errors.QuotaLimitExceededf("mailbox full"),
	code:       params.CodeQuotaLimitExceeded,
	status:     http.StatusInternalServerError,
	helperFunc: params.IsCodeQuotaLimitExceeded,
}, {
	err: &params.IncompatibleClientError{
		ServerVersion: jujuversion.Current,
	},
	code:   params.CodeIncompatibleClient,
	status: http.StatusInternalServerError,
	helperFunc: func(err error) bool {
		err1, ok := err.(*params.Error)
		err2 := &params.IncompatibleClientError{
			ServerVersion: jujuversion.Current,
		}
		if !ok || err1.Info == nil || !reflect.DeepEqual(err1.Info, err2.AsMap()) {
			return false
		}
		return true
	},
}, {
	err:    nil,
	code:   "",
	status: http.StatusOK,
}}

var sampleMacaroon = func() *macaroon.Macaroon {
	m, err := macaroon.New([]byte("key"), []byte("id"), "loc", macaroon.LatestVersion)
	if err != nil {
		panic(err)
	}
	return m
}()

var sampleRedirectError = func() *apiservererrors.RedirectError {
	hps, _ := network.ParseProviderHostPorts("1.1.1.1:12345", "2.2.2.2:7337")
	return &apiservererrors.RedirectError{
		Servers: []network.ProviderHostPorts{hps},
		CACert:  testing.ServerCert,
	}
}()

func asMap(v interface{}) map[string]interface{} {
	var m map[string]interface{}
	d, _ := json.Marshal(v)
	_ = json.Unmarshal(d, &m)

	return m
}

type unhashableError []string

func (err unhashableError) Error() string {
	return err[0]
}

func (s *errorsSuite) TestErrorTransform(c *gc.C) {
	for i, t := range errorTransformTests {
		c.Logf("running test %d: %T{%q}", i, t.err, t.err)
		err1, status := apiservererrors.ServerErrorAndStatus(t.err)

		// Sanity check that ServerError returns the same thing.
		err2 := apiservererrors.ServerError(t.err)
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

		// TODO(ericsnow) Remove this switch once the other error types are supported.
		switch t.code {
		case params.CodeHasAssignedUnits,
			params.CodeNoAddressSet,
			params.CodeUpgradeInProgress,
			params.CodeMachineHasAttachedStorage,
			params.CodeDischargeRequired,
			params.CodeModelNotFound,
			params.CodeRedirect,
			params.CodeIncompatibleClient:
			continue
		case params.CodeOperationBlocked:
			// ServerError doesn't actually have a case for this code.
			continue
		}

		c.Logf("  checking restore (%#v)", err1)
		restored := apiservererrors.RestoreError(err1)
		if t.err == nil {
			c.Check(restored, jc.ErrorIsNil)
		} else if t.code == "" {
			c.Check(restored.Error(), gc.Equals, t.err.Error())
		} else {
			// TODO(ericsnow) Use a stricter DeepEquals check.
			c.Check(errors.Cause(restored), gc.FitsTypeOf, t.err)
			c.Check(restored.Error(), gc.Equals, t.err.Error())
		}
	}
}

func (s *errorsSuite) TestUnknownModel(c *gc.C) {
	err := apiservererrors.UnknownModelError("dead-beef")
	c.Check(err, gc.ErrorMatches, `unknown model: "dead-beef"`)
}

func (s *errorsSuite) TestDestroyErr(c *gc.C) {
	errs := []error{
		errors.New("error one"),
		errors.New("error two"),
		errors.New("error three"),
	}
	ids := []string{
		"id1",
		"id2",
		"id3",
	}

	c.Assert(apiservererrors.DestroyErr("entities", ids, nil), jc.ErrorIsNil)

	err := apiservererrors.DestroyErr("entities", ids, errs)
	c.Assert(err, gc.ErrorMatches, "no entities were destroyed: error one; error two; error three")

	err = apiservererrors.DestroyErr("entities", ids, errs[1:])
	c.Assert(err, gc.ErrorMatches, "some entities were not destroyed: error two; error three")
}
