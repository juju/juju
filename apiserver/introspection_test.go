// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"context"
	"io"
	"net/http"
	"net/url"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	apitesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/access/service"
	"github.com/juju/juju/internal/auth"
	"github.com/juju/juju/juju/testing"
)

type introspectionSuite struct {
	testing.ApiServerSuite
	url string
}

var _ = tc.Suite(&introspectionSuite{})

func (s *introspectionSuite) SetUpTest(c *tc.C) {
	s.WithIntrospection = func(f func(path string, h http.Handler)) {
		f("navel", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, "gazing")
		}))
	}
	s.ApiServerSuite.SetUpTest(c)
	s.url = s.URL("/introspection/navel", url.Values{}).String()
}

func (s *introspectionSuite) TestAccess(c *tc.C) {
	s.testAccess(c, testing.AdminUser.String(), testing.AdminSecret)

	accessService := s.ControllerDomainServices(c).Access()
	userTag := names.NewUserTag("bobbrown")
	_, _, err := accessService.AddUser(context.Background(), service.AddUserArg{
		Name:        user.NameFromTag(userTag),
		DisplayName: "Bob Brown",
		CreatorUUID: s.AdminUserUUID,
		Password:    ptr(auth.NewPassword("hunter2")),
		Permission: permission.AccessSpec{
			Access: permission.LoginAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        s.ControllerUUID,
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	_, err = accessService.CreatePermission(context.Background(), permission.UserAccessSpec{
		AccessSpec: permission.AccessSpec{
			Target: permission.ID{
				ObjectType: permission.Model,
				Key:        s.ControllerModelUUID(),
			},
			Access: permission.ReadAccess,
		},
		User: user.NameFromTag(userTag),
	})
	c.Assert(err, jc.ErrorIsNil)

	s.testAccess(c, "user-bobbrown", "hunter2")
}

func (s *introspectionSuite) TestAccessDenied(c *tc.C) {
	accessService := s.ControllerDomainServices(c).Access()
	userTag := names.NewUserTag("bobbrown")
	_, _, err := accessService.AddUser(context.Background(), service.AddUserArg{
		Name:        user.NameFromTag(userTag),
		DisplayName: "Bob Brown",
		CreatorUUID: s.AdminUserUUID,
		Password:    ptr(auth.NewPassword("hunter2")),
		Permission: permission.AccessSpec{
			Access: permission.LoginAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        s.ControllerUUID,
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:   "GET",
		URL:      s.url,
		Tag:      userTag.String(),
		Password: "hunter2",
	})
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, tc.Equals, http.StatusForbidden)
}

func (s *introspectionSuite) testAccess(c *tc.C, tag, password string) {
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:   "GET",
		URL:      s.url,
		Tag:      tag,
		Password: password,
	})
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, tc.Equals, http.StatusOK)
	content, err := io.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(content), tc.Equals, "gazing")
}
