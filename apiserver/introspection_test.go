// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"context"
	"io"
	"net/http"
	"net/url"

	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apitesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/domain/user/service"
	"github.com/juju/juju/internal/auth"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type introspectionSuite struct {
	testing.ApiServerSuite
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
	s.url = s.URL("/introspection/navel", url.Values{}).String()
}

func (s *introspectionSuite) TestAccess(c *gc.C) {
	userService := s.ControllerServiceFactory(c).User()
	userTag := names.NewUserTag("bobbrown")
	_, _, err := userService.AddUser(context.Background(), service.AddUserArg{
		Name:        userTag.Name(),
		DisplayName: "Bob Brown",
		CreatorUUID: s.AdminUserUUID,
		Password:    ptr(auth.NewPassword("hunter2")),
	})
	c.Assert(err, jc.ErrorIsNil)

	s.testAccess(c, testing.AdminUser.String(), testing.AdminSecret)

	// TODO (stickupkid): Permissions: This is only required to insert admin
	// permissions into the state, remove when permissions are written to state.
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f.MakeUser(c, &factory.UserParams{
		Name:   userTag.Name(),
		Access: permission.ReadAccess,
	})

	s.testAccess(c, "user-bobbrown", "hunter2")
}

func (s *introspectionSuite) TestAccessDenied(c *gc.C) {
	userService := s.ControllerServiceFactory(c).User()
	userTag := names.NewUserTag("bobbrown")
	_, _, err := userService.AddUser(context.Background(), service.AddUserArg{
		Name:        userTag.Name(),
		DisplayName: "Bob Brown",
		CreatorUUID: s.AdminUserUUID,
		Password:    ptr(auth.NewPassword("hunter2")),
	})
	c.Assert(err, jc.ErrorIsNil)

	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:   "GET",
		URL:      s.url,
		Tag:      "user-bobbrown",
		Password: "hunter2",
	})
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, gc.Equals, http.StatusUnauthorized)
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
