// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state/api"
)

var badLoginTests = []struct {
	tag      string
	password string
	err      string
	code     string
}{{
	tag:      "user-admin",
	password: "wrong password",
	err:      "invalid entity name or password",
	code:     api.CodeUnauthorized,
}, {
	tag:      "user-foo",
	password: "password",
	err:      "invalid entity name or password",
	code:     api.CodeUnauthorized,
}, {
	tag:      "bar",
	password: "password",
	err:      `invalid entity tag "bar"`,
}}

func (s *suite) TestBadLogin(c *C) {
	_, info, err := s.APIConn.Environ.StateInfo()
	c.Assert(err, IsNil)
	for i, t := range badLoginTests {
		c.Logf("test %d; entity %q; password %q", i, t.tag, t.password)
		info.Tag = ""
		info.Password = ""
		func() {
			st, err := api.Open(info)
			c.Assert(err, IsNil)
			defer st.Close()

			_, err = st.Machine("0")
			c.Assert(err, ErrorMatches, "not logged in")
			c.Assert(api.ErrCode(err), Equals, api.CodeUnauthorized, Commentf("error %#v", err))

			_, err = st.Unit("foo/0")
			c.Assert(err, ErrorMatches, "not logged in")
			c.Assert(api.ErrCode(err), Equals, api.CodeUnauthorized)

			err = st.Login(t.tag, t.password)
			c.Assert(err, ErrorMatches, t.err)
			c.Assert(api.ErrCode(err), Equals, t.code)

			_, err = st.Machine("0")
			c.Assert(err, ErrorMatches, "not logged in")
			c.Assert(api.ErrCode(err), Equals, api.CodeUnauthorized)
		}()
	}
}
