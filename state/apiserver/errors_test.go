// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	stderrors "errors"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
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
	code: api.CodeNotFound,
}, {
	err:  errors.Unauthorizedf("hello"),
	code: api.CodeUnauthorized,
}, {
	err:  state.ErrCannotEnterScopeYet,
	code: api.CodeCannotEnterScopeYet,
}, {
	err:  state.ErrCannotEnterScope,
	code: api.CodeCannotEnterScope,
}, {
	err:  state.ErrExcessiveContention,
	code: api.CodeExcessiveContention,
}, {
	err:  state.ErrUnitHasSubordinates,
	code: api.CodeUnitHasSubordinates,
}, {
	err:  common.ErrBadId,
	code: api.CodeNotFound,
}, {
	err:  common.ErrBadCreds,
	code: api.CodeUnauthorized,
}, {
	err:  common.ErrPerm,
	code: api.CodeUnauthorized,
}, {
	err:  common.ErrNotLoggedIn,
	code: api.CodeUnauthorized,
}, {
	err:  common.ErrUnknownWatcher,
	code: api.CodeNotFound,
}, {
	err:  &state.NotAssignedError{&state.Unit{}}, // too sleazy?! nah..
	code: api.CodeNotAssigned,
}, {
	err:  common.ErrStoppedWatcher,
	code: api.CodeStopped,
}, {
	err:  &state.HasAssignedUnitsError{"42", []string{"a"}},
	code: api.CodeHasAssignedUnits,
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
