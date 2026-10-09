// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	apitesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/permission"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/access/service"
	applicationservice "github.com/juju/juju/domain/application/service"
	internalcharm "github.com/juju/juju/domain/deployment/charm"
	charmresource "github.com/juju/juju/domain/deployment/charm/resource"
	"github.com/juju/juju/domain/resource"
	"github.com/juju/juju/internal/auth"
	"github.com/juju/juju/internal/docker"
	internalpassword "github.com/juju/juju/internal/password"
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

// TestUnitAgentDownloadsOwnResource checks that a unit agent, authenticating
// with its own tag and password as the uniter does, gets the resource of its
// own unit back through the endpoint.
func (s *unitResourcesAuthSuite) TestUnitAgentDownloadsOwnResource(c *tc.C) {
	blob, password := s.deployUnitsWithResource(c)

	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:   "GET",
		URL:      s.unitResourcesURL("unit-wordpress-0", "blob"),
		Tag:      "unit-wordpress-0",
		Password: password,
	})
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(resp.StatusCode, tc.Equals, http.StatusOK, tc.Commentf("body: %s", body))
	c.Check(body, tc.DeepEquals, blob)
}

// TestUnitAgentRefusedForPeerUnit checks that a unit agent is refused the
// resource of another unit, even one of its own application. The agent gets
// past the endpoint authorizer, so the refusal comes from the opener getter as
// ErrPerm, which the API server reports as 401 the way it does for facades.
func (s *unitResourcesAuthSuite) TestUnitAgentRefusedForPeerUnit(c *tc.C) {
	_, password := s.deployUnitsWithResource(c)

	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:   "GET",
		URL:      s.unitResourcesURL("unit-wordpress-1", "blob"),
		Tag:      "unit-wordpress-0",
		Password: password,
	})
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(resp.StatusCode, tc.Equals, http.StatusUnauthorized)
	c.Check(string(body), tc.Contains, "permission denied")
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

// deployUnitsWithResource deploys wordpress from a local charm with two units
// and stores the blob of its one resource, then gives wordpress/0 a password
// so that it can authenticate as a unit agent. The blob and the password are
// returned.
//
// The resource is a container image rather than a file because the domain
// services of the test API server are wired to a no-op object store, while
// container image blobs are kept in the model database and so can be read
// back through the endpoint.
func (s *unitResourcesAuthSuite) deployUnitsWithResource(c *tc.C) ([]byte, string) {
	ctx := c.Context()
	domainServices := s.ControllerDomainServices(c)

	base, err := internalcharm.ParseBase("ubuntu@22.04", arch.DefaultArchitecture)
	c.Assert(err, tc.ErrorIsNil)
	charm := internalcharm.NewCharmBase(
		&internalcharm.Meta{
			Name: "wordpress",
			Resources: map[string]charmresource.Meta{
				"blob": {
					Name: "blob",
					Type: charmresource.TypeContainerImage,
					Path: "blob",
				},
			},
		},
		&internalcharm.Manifest{Bases: []internalcharm.Base{base}},
		nil, nil,
	)
	origin := corecharm.Origin{
		Source:   corecharm.Local,
		Revision: new(1),
		Platform: corecharm.Platform{
			Architecture: arch.DefaultArchitecture,
			OS:           "ubuntu",
			Channel:      "22.04",
		},
	}
	appUUID, err := domainServices.Application().CreateIAASApplication(
		ctx, "wordpress", charm, origin,
		applicationservice.AddApplicationArgs{
			ReferenceName: "wordpress",
			ResolvedResources: applicationservice.ResolvedResources{{
				Name:   "blob",
				Origin: charmresource.OriginUpload,
			}},
		},
		applicationservice.AddIAASUnitArg{},
		applicationservice.AddIAASUnitArg{},
	)
	c.Assert(err, tc.ErrorIsNil)

	password, err := internalpassword.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)
	err = domainServices.AgentPassword().SetUnitPassword(
		ctx, coreunit.Name("wordpress/0"), password,
	)
	c.Assert(err, tc.ErrorIsNil)

	blob, err := json.Marshal(docker.DockerImageDetails{
		RegistryPath: "example.com/wordpress@sha256:abc123",
		ImageRepoDetails: docker.ImageRepoDetails{
			BasicAuthConfig: docker.BasicAuthConfig{
				Username: "wordpress",
				Password: "hunter2",
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	fingerprint, err := charmresource.GenerateFingerprint(bytes.NewReader(blob))
	c.Assert(err, tc.ErrorIsNil)

	resourceService := domainServices.Resource()
	resourceUUID, err := resourceService.GetApplicationResourceID(
		ctx, resource.GetApplicationResourceIDArgs{
			ApplicationUUID: appUUID,
			Name:            "blob",
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	_, err = resourceService.StoreResource(ctx, resource.StoreResourceArgs{
		ResourceUUID: resourceUUID,
		Reader:       bytes.NewReader(blob),
		Size:         int64(len(blob)),
		Fingerprint:  fingerprint,
	})
	c.Assert(err, tc.ErrorIsNil)

	return blob, password
}
