// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"io"
	"net/http"
	"net/url"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apitesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
)

type introspectionSuite struct {
	testing.ApiServerSuite
	bob *state.User
	url string
}

var _ = gc.Suite(&introspectionSuite{})

func (s *introspectionSuite) SetUpTest(c *gc.C) {
	s.WithIntrospection = func(f func(path string, h http.Handler)) {
		f("navel", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, "gazing")
		}))
	}
	s.ApiServerSuite.SetUpTest(c)
	bob, err := s.ControllerModel(c).State().AddUser("bob", "", "hunter2", "admin")
	c.Assert(err, jc.ErrorIsNil)
	s.bob = bob
	s.url = s.URL("/introspection/navel", url.Values{}).String()
}

func (s *introspectionSuite) TestAccess(c *gc.C) {
	s.testAccess(c, testing.AdminUser.String(), testing.AdminSecret)
	_, err := s.ControllerModel(c).AddUser(
		state.UserAccessSpec{
			User:      s.bob.UserTag(),
			CreatedBy: testing.AdminUser,
			Access:    permission.ReadAccess,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	s.testAccess(c, "user-bob", "hunter2")
}

func (s *introspectionSuite) testAccess(c *gc.C, tag, password string) {
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:   "GET",
		URL:      s.url,
		Tag:      tag,
		Password: password,
	})
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, gc.Equals, http.StatusOK)
	content, err := io.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(content), gc.Equals, "gazing")
}

func (s *introspectionSuite) TestAccessDenied(c *gc.C) {
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:   "GET",
		URL:      s.url,
		Tag:      "user-bob",
		Password: "hunter2",
	})
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, gc.Equals, http.StatusForbidden)
}
