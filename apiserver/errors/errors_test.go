// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors_test

import (
	"encoding/json"
	stderrors "errors"
	"net/http"
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn/v3"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/network"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
	stateerrors "github.com/juju/juju/state/errors"
)

type errorsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&errorsSuite{})

var errorTransformTests = []struct {
	err          error
	code         string
	status       int
	helperFunc   func(error) bool
	targetTester func(error) bool
}{{
	err:        errors.NotFound,
	code:       params.CodeNotFound,
	status:     http.StatusNotFound,
	helperFunc: params.IsCodeNotFound,
}, {
	err:        errors.UserNotFound,
	code:       params.CodeUserNotFound,
	status:     http.StatusNotFound,
	helperFunc: params.IsCodeUserNotFound,
}, {
	err:        secreterrors.SecretNotFound,
	code:       params.CodeSecretNotFound,
	status:     http.StatusNotFound,
	helperFunc: params.IsCodeSecretNotFound,
}, {
	err:        secreterrors.SecretRevisionNotFound,
	code:       params.CodeSecretRevisionNotFound,
	status:     http.StatusNotFound,
	helperFunc: params.IsCodeSecretRevisionNotFound,
}, {
	err:        secreterrors.SecretConsumerNotFound,
	code:       params.CodeSecretConsumerNotFound,
	status:     http.StatusNotFound,
	helperFunc: params.IsCodeSecretConsumerNotFound,
}, {
	err:        secretbackenderrors.NotFound,
	code:       params.CodeSecretBackendNotFound,
	status:     http.StatusNotFound,
	helperFunc: params.IsCodeSecretBackendNotFound,
}, {
	err:        secretbackenderrors.Forbidden,
	code:       params.CodeSecretBackendForbidden,
	status:     http.StatusForbidden,
	helperFunc: params.IsCodeSecretBackendForbidden,
}, {
	err:        errors.Unauthorized,
	code:       params.CodeUnauthorized,
	status:     http.StatusUnauthorized,
	helperFunc: params.IsCodeUnauthorized,
}, {
	err:        stateerrors.ErrDead,
	code:       params.CodeDead,
	status:     http.StatusInternalServerError,
	helperFunc: params.IsCodeDead,
}, {
	err:        jujutxn.ErrExcessiveContention,
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
	err:        apiservererrors.NewNoAddressSetError(names.NewUnitTag("mysql/0"), "public"),
	code:       params.CodeNoAddressSet,
	status:     http.StatusInternalServerError,
	helperFunc: params.IsCodeNoAddressSet,
	targetTester: func(e error) bool {
		return errors.Is(e, apiservererrors.NoAddressSetError)
	},
}, {
	err:        apiservererrors.ErrUnauthorized,
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
	err:        errors.NotProvisioned,
	code:       params.CodeNotProvisioned,
	status:     http.StatusInternalServerError,
	helperFunc: params.IsCodeNotProvisioned,
}, {
	err:        errors.AlreadyExists,
	code:       params.CodeAlreadyExists,
	status:     http.StatusInternalServerError,
	helperFunc: params.IsCodeAlreadyExists,
}, {
	err:        apiservererrors.ErrUnknownWatcher,
	code:       params.CodeNotFound,
	status:     http.StatusNotFound,
	helperFunc: params.IsCodeNotFound,
}, {
	err:        errors.NotAssigned,
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
	targetTester: func(e error) bool {
		return errors.Is(e, stateerrors.HasAssignedUnitsError)
	},
}, {
	err:        apiservererrors.ErrTryAgain,
	code:       params.CodeTryAgain,
	status:     http.StatusInternalServerError,
	helperFunc: params.IsCodeTryAgain,
}, {
	err:        errors.ConstError(leadership.ErrClaimDenied),
	code:       params.CodeLeadershipClaimDenied,
	status:     http.StatusInternalServerError,
	helperFunc: params.IsCodeLeadershipClaimDenied,
}, {
	err:        errors.ConstError(lease.ErrClaimDenied),
	code:       params.CodeLeaseClaimDenied,
	status:     http.StatusInternalServerError,
	helperFunc: params.IsCodeLeaseClaimDenied,
}, {
	err:        apiservererrors.OperationBlockedError("test"),
	code:       params.CodeOperationBlocked,
	status:     http.StatusBadRequest,
	helperFunc: params.IsCodeOperationBlocked,
	targetTester: func(e error) bool {
		return errors.HasType[*params.Error](e)
	},
}, {
	err:        errors.NotSupported,
	code:       params.CodeNotSupported,
	status:     http.StatusInternalServerError,
	helperFunc: params.IsCodeNotSupported,
}, {
	err:        errors.BadRequest,
	code:       params.CodeBadRequest,
	status:     http.StatusBadRequest,
	helperFunc: params.IsBadRequest,
}, {
	err:        errors.MethodNotAllowed,
	code:       params.CodeMethodNotAllowed,
	status:     http.StatusMethodNotAllowed,
	helperFunc: params.IsMethodNotAllowed,
}, {
	err:    stderrors.New("an error"),
	status: http.StatusInternalServerError,
	code:   "",
	targetTester: func(e error) bool {
		return e.Error() == "an error"
	},
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
	targetTester: func(e error) bool {
		return errors.HasType[*apiservererrors.DischargeRequiredError](e)
	},
}, {
	err:    unhashableError{"foo"},
	status: http.StatusInternalServerError,
	code:   "",
	targetTester: func(e error) bool {
		return e.Error() == "foo"
	},
}, {
	err:        apiservererrors.UnknownModelError,
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
	err:        errors.QuotaLimitExceeded,
	code:       params.CodeQuotaLimitExceeded,
	status:     http.StatusInternalServerError,
	helperFunc: params.IsCodeQuotaLimitExceeded,
}, {
	err:        errors.NotYetAvailable,
	code:       params.CodeNotYetAvailable,
	status:     http.StatusConflict,
	helperFunc: params.IsCodeNotYetAvailable,
}, {
	err:    apiservererrors.NewNotLeaderError("1.1.1.1", "1"),
	code:   params.CodeNotLeader,
	status: http.StatusTemporaryRedirect,
	targetTester: func(e error) bool {
		return errors.HasType[*apiservererrors.NotLeaderError](e)
	},
}, {
	err:    apiservererrors.DeadlineExceededError,
	code:   params.CodeDeadlineExceeded,
	status: http.StatusInternalServerError,
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
			params.CodeRedirect:
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
		}

		if t.targetTester == nil {
			c.Check(restored, jc.ErrorIs, t.err)
			c.Check(restored.Error(), gc.Equals, t.err.Error())
		} else {
			c.Check(t.targetTester(restored), jc.IsTrue)
			c.Check(restored.Error(), gc.Equals, t.err.Error())
		}
	}
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
