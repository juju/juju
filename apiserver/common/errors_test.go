// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	stderrors "errors"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/txn"
	gc "gopkg.in/check.v1"

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
	helperFunc func(error) bool
}{{
	err:        errors.NotFoundf("hello"),
	code:       params.CodeNotFound,
	helperFunc: params.IsCodeNotFound,
}, {
	err:        errors.Unauthorizedf("hello"),
	code:       params.CodeUnauthorized,
	helperFunc: params.IsCodeUnauthorized,
}, {
	err:        state.ErrCannotEnterScopeYet,
	code:       params.CodeCannotEnterScopeYet,
	helperFunc: params.IsCodeCannotEnterScopeYet,
}, {
	err:        state.ErrCannotEnterScope,
	code:       params.CodeCannotEnterScope,
	helperFunc: params.IsCodeCannotEnterScope,
}, {
	err:        state.ErrDead,
	code:       params.CodeDead,
	helperFunc: params.IsCodeDead,
}, {
	err:        txn.ErrExcessiveContention,
	code:       params.CodeExcessiveContention,
	helperFunc: params.IsCodeExcessiveContention,
}, {
	err:        state.ErrUnitHasSubordinates,
	code:       params.CodeUnitHasSubordinates,
	helperFunc: params.IsCodeUnitHasSubordinates,
}, {
	err:        common.ErrBadId,
	code:       params.CodeNotFound,
	helperFunc: params.IsCodeNotFound,
}, {
	err:        common.NoAddressSetError(names.NewUnitTag("mysql/0"), "public"),
	code:       params.CodeNoAddressSet,
	helperFunc: params.IsCodeNoAddressSet,
}, {
	err:        common.ErrBadCreds,
	code:       params.CodeUnauthorized,
	helperFunc: params.IsCodeUnauthorized,
}, {
	err:        common.ErrPerm,
	code:       params.CodeUnauthorized,
	helperFunc: params.IsCodeUnauthorized,
}, {
	err:        common.ErrNotLoggedIn,
	code:       params.CodeUnauthorized,
	helperFunc: params.IsCodeUnauthorized,
}, {
	err:        errors.NotProvisionedf("machine 0"),
	code:       params.CodeNotProvisioned,
	helperFunc: params.IsCodeNotProvisioned,
}, {
	err:        errors.AlreadyExistsf("blah"),
	code:       params.CodeAlreadyExists,
	helperFunc: params.IsCodeAlreadyExists,
}, {
	err:        common.ErrUnknownWatcher,
	code:       params.CodeNotFound,
	helperFunc: params.IsCodeNotFound,
}, {
	err:        errors.NotAssignedf("unit mysql/0"),
	code:       params.CodeNotAssigned,
	helperFunc: params.IsCodeNotAssigned,
}, {
	err:        common.ErrStoppedWatcher,
	code:       params.CodeStopped,
	helperFunc: params.IsCodeStopped,
}, {
	err:        &state.HasAssignedUnitsError{"42", []string{"a"}},
	code:       params.CodeHasAssignedUnits,
	helperFunc: params.IsCodeHasAssignedUnits,
}, {
	err:        common.ErrTryAgain,
	code:       params.CodeTryAgain,
	helperFunc: params.IsCodeTryAgain,
}, {
	err:        state.UpgradeInProgressError,
	code:       params.CodeUpgradeInProgress,
	helperFunc: params.IsCodeUpgradeInProgress,
}, {
	err:        leadership.ErrClaimDenied,
	code:       params.CodeLeadershipClaimDenied,
	helperFunc: params.IsCodeLeadershipClaimDenied,
}, {
	err:        common.ErrOperationBlocked("test"),
	code:       params.CodeOperationBlocked,
	helperFunc: params.IsCodeOperationBlocked,
}, {
	err:        errors.NotSupportedf("needed feature"),
	code:       params.CodeNotSupported,
	helperFunc: params.IsCodeNotSupported,
}, {
	err:  stderrors.New("an error"),
	code: "",
}, {
	err:  unhashableError{"foo"},
	code: "",
}, {
	err:        common.UnknownEnvironmentError("dead-beef-123456"),
	code:       params.CodeNotFound,
	helperFunc: params.IsCodeNotFound,
}, {
	err:  nil,
	code: "",
}}

type unhashableError []string

func (err unhashableError) Error() string {
	return err[0]
}

func (s *errorsSuite) TestErrorTransform(c *gc.C) {
	for i, t := range errorTransformTests {
		c.Logf("running test %d: %T{%q}", i, t.err, t.err)

		err1 := common.ServerError(t.err)
		if t.err == nil {
			c.Assert(err1, gc.IsNil)
		} else {
			c.Assert(err1.Message, gc.Equals, t.err.Error())
			c.Assert(err1.Code, gc.Equals, t.code)
			if t.helperFunc != nil {
				c.Assert(err1, jc.Satisfies, t.helperFunc)
			}
		}

		// TODO(ericsnow) Remove this switch once the other error types are supported.
		switch t.code {
		case params.CodeHasAssignedUnits,
			params.CodeNoAddressSet,
			params.CodeUpgradeInProgress,
			params.CodeMachineHasAttachedStorage:
			continue
		case params.CodeNotFound:
			if common.IsUnknownEnviromentError(t.err) {
				continue
			}
		case params.CodeOperationBlocked:
			// ServerError doesn't actually have a case for this code.
			continue
		}

		c.Logf("  checking restore (%#v)", err1)
		restored, ok := common.RestoreError(err1)
		if t.err == nil {
			c.Check(ok, jc.IsTrue)
			c.Check(restored, jc.ErrorIsNil)
		} else if t.code == "" {
			c.Check(ok, jc.IsFalse)
			c.Check(restored.Error(), gc.Equals, t.err.Error())
		} else {
			c.Check(ok, jc.IsTrue)
			// TODO(ericsnow) Use a stricter DeepEquals check.
			c.Check(errors.Cause(restored), gc.FitsTypeOf, t.err)
			c.Check(restored.Error(), gc.Equals, t.err.Error())
		}
	}
}

func (s *errorsSuite) TestUnknownEnvironment(c *gc.C) {
	err := common.UnknownEnvironmentError("dead-beef")
	c.Check(err, gc.ErrorMatches, `unknown environment: "dead-beef"`)
}
