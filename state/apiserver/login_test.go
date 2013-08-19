// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	gc "launchpad.net/gocheck"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver"
	coretesting "launchpad.net/juju-core/testing"
)

type loginSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&loginSuite{})

var badLoginTests = []struct {
	tag      string
	password string
	err      string
	code     string
}{{
	tag:      "user-admin",
	password: "wrong password",
	err:      "invalid entity name or password",
	code:     params.CodeUnauthorized,
}, {
	tag:      "user-foo",
	password: "password",
	err:      "invalid entity name or password",
	code:     params.CodeUnauthorized,
}, {
	tag:      "bar",
	password: "password",
	err:      `"bar" is not a valid tag`,
}}

func (s *loginSuite) TestBadLogin(c *gc.C) {
	// Start our own server so we can control when the first login
	// happens. Otherwise in JujuConnSuite.SetUpTest api.Open is
	// called with user-admin permissions automatically.
	srv, err := apiserver.NewServer(
		s.State,
		"localhost:0",
		[]byte(coretesting.ServerCert),
		[]byte(coretesting.ServerKey),
	)
	c.Assert(err, gc.IsNil)
	defer func() {
		err := srv.Stop()
		c.Assert(err, gc.IsNil)
	}()

	info := &api.Info{
		Tag:      "",
		Password: "",
		Addrs:    []string{srv.Addr()},
		CACert:   []byte(coretesting.CACert),
	}
	for i, t := range badLoginTests {
		c.Logf("test %d; entity %q; password %q", i, t.tag, t.password)
		// Note that Open does not log in if the tag and password
		// are empty. This allows us to test operations on the connection
		// before calling Login, which we could not do if Open
		// always logged in.
		info.Tag = ""
		info.Password = ""
		func() {
			st, err := api.Open(info, fastDialOpts)
			c.Assert(err, gc.IsNil)
			defer st.Close()

			_, err = st.Machiner().Machine("0")
			c.Assert(err, gc.ErrorMatches, `unknown object type "Machiner"`)

			// Since these are user login tests, the nonce is empty.
			err = st.Login(t.tag, t.password, "")
			c.Assert(err, gc.ErrorMatches, t.err)
			c.Assert(params.ErrCode(err), gc.Equals, t.code)

			_, err = st.Machiner().Machine("0")
			c.Assert(err, gc.ErrorMatches, `unknown object type "Machiner"`)
		}()
	}
}
