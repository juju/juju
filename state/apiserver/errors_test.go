// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"errors"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/apiserver"
)

var errorTransformTests = []struct {
	err  error
	code string
}{{
	err:  state.NotFoundf("hello"),
	code: api.CodeNotFound,
}, {
	err:  state.Unauthorizedf("hello"),
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
	err:  apiserver.ErrBadId,
	code: api.CodeNotFound,
}, {
	err:  apiserver.ErrBadCreds,
	code: api.CodeUnauthorized,
}, {
	err:  apiserver.ErrPerm,
	code: api.CodeUnauthorized,
}, {
	err:  apiserver.ErrNotLoggedIn,
	code: api.CodeUnauthorized,
}, {
	err:  apiserver.ErrUnknownWatcher,
	code: api.CodeNotFound,
}, {
	err:  &state.NotAssignedError{&state.Unit{}}, // too sleazy?! nah..
	code: api.CodeNotAssigned,
}, {
	err:  apiserver.ErrStoppedWatcher,
	code: api.CodeStopped,
}, {
	err:  &state.HasAssignedUnitsError{"42", []string{"a"}},
	code: api.CodeHasAssignedUnits,
}, {
	err:  errors.New("an error"),
	code: "",
}}

func (s *suite) TestErrorTransform(c *C) {
	for _, t := range errorTransformTests {
		err1 := apiserver.ServerError(t.err)
		c.Assert(err1.Error(), Equals, t.err.Error())
		if t.code != "" {
			c.Assert(api.ErrCode(err1), Equals, t.code)
		} else {
			c.Assert(err1, Equals, t.err)
		}
	}
}

func (s *suite) TestErrors(c *C) {
	stm, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	setDefaultPassword(c, stm)
	st := s.openAs(c, stm.Tag())
	defer st.Close()
	// By testing this single call, we test that the
	// error transformation function is correctly called
	// on error returns from the API apiserver. The transformation
	// function itself is tested below.
	_, err = st.Machine("99")
	c.Assert(api.ErrCode(err), Equals, api.CodeNotFound)
}
