// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	stderrors "errors"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	"launchpad.net/juju-core/testing"
)

type errorsSuite struct {
	testing.LoggingSuite
}

var _ = Suite(&errorsSuite{})

var errorTransformTests = []struct {
	err  error
	code string
}{{
	err:  errors.NotFoundf("hello"),
	code: params.CodeNotFound,
}, {
	err:  errors.Unauthorizedf("hello"),
	code: params.CodeUnauthorized,
}, {
	err:  state.ErrCannotEnterScopeYet,
	code: params.CodeCannotEnterScopeYet,
}, {
	err:  state.ErrCannotEnterScope,
	code: params.CodeCannotEnterScope,
}, {
	err:  state.ErrExcessiveContention,
	code: params.CodeExcessiveContention,
}, {
	err:  state.ErrUnitHasSubordinates,
	code: params.CodeUnitHasSubordinates,
}, {
	err:  common.ErrBadId,
	code: params.CodeNotFound,
}, {
	err:  common.ErrBadCreds,
	code: params.CodeUnauthorized,
}, {
	err:  common.ErrPerm,
	code: params.CodeUnauthorized,
}, {
	err:  common.ErrNotLoggedIn,
	code: params.CodeUnauthorized,
}, {
	err:  common.ErrNotProvisioned,
	code: params.CodeNotProvisioned,
}, {
	err:  common.ErrUnknownWatcher,
	code: params.CodeNotFound,
}, {
	err:  &errors.NotAssignedError{"unit-mysql-0"},
	code: params.CodeNotAssigned,
}, {
	err:  common.ErrStoppedWatcher,
	code: params.CodeStopped,
}, {
	err:  &state.HasAssignedUnitsError{"42", []string{"a"}},
	code: params.CodeHasAssignedUnits,
}, {
	err:  stderrors.New("an error"),
	code: "",
}, {
	err:  nil,
	code: "",
}}

func (s *errorsSuite) TestErrorTransform(c *C) {
	for _, t := range errorTransformTests {
		err1 := common.ServerError(t.err)
		if t.err == nil {
			c.Assert(err1, IsNil)
		} else {
			c.Assert(err1.Message, Equals, t.err.Error())
			c.Assert(err1.Code, Equals, t.code)
		}
	}
}
