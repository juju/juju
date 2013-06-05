// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	stderrors "errors"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	apicommon "launchpad.net/juju-core/state/apiserver/common"
)

type errorsSuite struct {
	baseSuite
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
	err:  apicommon.ErrBadId,
	code: api.CodeNotFound,
}, {
	err:  apicommon.ErrBadCreds,
	code: api.CodeUnauthorized,
}, {
	err:  apicommon.ErrPerm,
	code: api.CodeUnauthorized,
}, {
	err:  apicommon.ErrNotLoggedIn,
	code: api.CodeUnauthorized,
}, {
	err:  apicommon.ErrUnknownWatcher,
	code: api.CodeNotFound,
}, {
	err:  &state.NotAssignedError{&state.Unit{}}, // too sleazy?! nah..
	code: api.CodeNotAssigned,
}, {
	err:  apicommon.ErrStoppedWatcher,
	code: api.CodeStopped,
}, {
	err:  &state.HasAssignedUnitsError{"42", []string{"a"}},
	code: api.CodeHasAssignedUnits,
}, {
	err:  stderrors.New("an error"),
	code: "",
}}

func (s *errorsSuite) TestErrorTransform(c *C) {
	for _, t := range errorTransformTests {
		err1 := apicommon.ServerError(t.err)
		c.Assert(err1.Error(), Equals, t.err.Error())
		if t.code != "" {
			c.Assert(api.ErrCode(err1), Equals, t.code)
		} else {
			c.Assert(err1, Equals, t.err)
		}
	}
}
