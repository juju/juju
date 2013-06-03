// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/apiserver"
	coretesting "launchpad.net/juju-core/testing"
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
	// Start our own server so we can control when the first login
	// happens. Otherwise in JujuConnSuite.SetUpTest api.Open is
	// called with user-admin permissions automatically.
	srv, err := apiserver.NewServer(
		s.State,
		"localhost:0",
		[]byte(coretesting.ServerCert),
		[]byte(coretesting.ServerKey),
	)
	c.Assert(err, IsNil)
	defer func() {
		err := srv.Stop()
		c.Assert(err, IsNil)
	}()

	info := &api.Info{
		Tag:      "",
		Password: "",
		Addrs:    []string{srv.Addr()},
		CACert:   []byte(coretesting.CACert),
	}
	for i, t := range badLoginTests {
		c.Logf("test %d; entity %q; password %q", i, t.tag, t.password)
		info.Tag = ""
		info.Password = ""
		func() {
			st, err := api.Open(info)
			c.Assert(err, IsNil)
			defer st.Close()

			_, err = st.Machiner()
			c.Assert(err, ErrorMatches, "not logged in")
			c.Assert(api.ErrCode(err), Equals, api.CodeUnauthorized, Commentf("error %#v", err))

			err = st.Login(t.tag, t.password)
			c.Assert(err, ErrorMatches, t.err)
			c.Assert(api.ErrCode(err), Equals, t.code)

			_, err = st.Machiner()
			c.Assert(err, ErrorMatches, "not logged in")
			c.Assert(api.ErrCode(err), Equals, api.CodeUnauthorized)
		}()
	}
}
