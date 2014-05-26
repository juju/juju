// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	stderrors "errors"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	"launchpad.net/juju-core/testing"
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
	err:        state.ErrExcessiveContention,
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
	err:        common.NoAddressSetError("unit-mysql-0", "public"),
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
	err:        state.NotProvisionedError("0"),
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
	err:        &state.NotAssignedError{&state.Unit{}}, // too sleazy?! nah..
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
	err:  stderrors.New("an error"),
	code: "",
}, {
	err:  unhashableError{"foo"},
	code: "",
}, {
	err:  nil,
	code: "",
}}

type unhashableError []string

func (err unhashableError) Error() string {
	return err[0]
}

func (s *errorsSuite) TestErrorTransform(c *gc.C) {
	for _, t := range errorTransformTests {
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
	}
}
