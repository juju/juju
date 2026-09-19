// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"fmt"
	"net/http"
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	apitesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/access/service"
	"github.com/juju/juju/internal/auth"
	"github.com/juju/juju/juju/testing"
)

// unitResourcesAuthSuite checks who is let through to the unit resources
// download endpoint of a running API server.
type unitResourcesAuthSuite struct {
	testing.ApiServerSuite
}

func TestUnitResourcesAuthSuite(t *stdtesting.T) {
	tc.Run(t, &unitResourcesAuthSuite{})
}

func (s *unitResourcesAuthSuite) unitResourcesURL(unit, res string) string {
	return s.URL(fmt.Sprintf(
		"/model/%s/units/%s/resources/%s", s.ControllerModelUUID(), unit, res,
	), nil).String()
}

// TestUserWithoutModelAccessRefused checks that a user who can log in to the
// controller, but has no access to the model, is refused before the unit
// named in the URL is looked up in the model.
func (s *unitResourcesAuthSuite) TestUserWithoutModelAccessRefused(c *tc.C) {
	accessService := s.ControllerDomainServices(c).Access()
	userTag := names.NewUserTag("bobbrown")
	_, _, err := accessService.AddUser(c.Context(), service.AddUserArg{
		Name:        user.NameFromTag(userTag),
		DisplayName: "Bob Brown",
		CreatorUUID: s.AdminUserUUID,
		Password:    new(auth.NewPassword("hunter2")),
		Permission: permission.AccessSpec{
			Access: permission.LoginAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        s.ControllerUUID,
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:   "GET",
		URL:      s.unitResourcesURL("unit-wordpress-0", "blob"),
		Tag:      userTag.String(),
		Password: "hunter2",
	})
	defer resp.Body.Close()
	c.Check(resp.StatusCode, tc.Equals, http.StatusForbidden)
}

// TestAdminUserRefused checks that the endpoint is for agents only, as the
// ResourcesHookContext facade is. Users, whatever their access, download
// resources through the application resources endpoint instead.
func (s *unitResourcesAuthSuite) TestAdminUserRefused(c *tc.C) {
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:   "GET",
		URL:      s.unitResourcesURL("unit-wordpress-0", "blob"),
		Tag:      testing.AdminUser.String(),
		Password: testing.AdminSecret,
	})
	defer resp.Body.Close()
	c.Check(resp.StatusCode, tc.Equals, http.StatusForbidden)
}
