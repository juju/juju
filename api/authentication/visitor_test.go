// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication_test

import (
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"

	"github.com/juju/juju/api/authentication"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
)

type VisitorSuite struct {
	testing.IsolationSuite

	jar     *cookiejar.Jar
	client  *httpbakery.Client
	server  *httptest.Server
	handler http.Handler
}

var _ = gc.Suite(&VisitorSuite{})

func (s *VisitorSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	var err error
	s.jar, err = cookiejar.New(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.client = httpbakery.NewClient()
	s.client.Jar = s.jar
	s.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.handler.ServeHTTP(w, r)
	}))
	s.AddCleanup(func(c *gc.C) { s.server.Close() })
}

func (s *VisitorSuite) TestVisitWebPage(c *gc.C) {
	v := authentication.NewVisitor("bob", func(username string) (string, error) {
		c.Assert(username, gc.Equals, "bob")
		return "hunter2", nil
	})
	var formUser, formPassword string
	s.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		formUser = r.Form.Get("user")
		formPassword = r.Form.Get("password")
	})
	err := v.VisitWebPage(s.client, map[string]*url.URL{
		"juju_userpass": mustParseURL(s.server.URL),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(formUser, gc.Equals, "bob")
	c.Assert(formPassword, gc.Equals, "hunter2")
}

func (s *VisitorSuite) TestVisitWebPageMethodNotSupported(c *gc.C) {
	v := authentication.NewVisitor("bob", nil)
	err := v.VisitWebPage(s.client, map[string]*url.URL{})
	c.Assert(err, gc.Equals, httpbakery.ErrMethodNotSupported)
}

func (s *VisitorSuite) TestVisitWebPageErrorResult(c *gc.C) {
	v := authentication.NewVisitor("bob", func(username string) (string, error) {
		return "hunter2", nil
	})
	s.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"Message":"bleh"}`, http.StatusInternalServerError)
	})
	err := v.VisitWebPage(s.client, map[string]*url.URL{
		"juju_userpass": mustParseURL(s.server.URL),
	})
	c.Assert(err, gc.ErrorMatches, "bleh")
}

func mustParseURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}
