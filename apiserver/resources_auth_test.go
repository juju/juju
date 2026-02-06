// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strings"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"

	apitesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type resourcesAuthSuite struct {
	apiserverBaseSuite
}

func (s *resourcesAuthSuite) resourcesURL(app, res string) *url.URL {
	u := s.URL(fmt.Sprintf("/model/%s/applications/%s/resources/%s", s.Model.UUID(), app, res), nil)
	return u
}

func (s *resourcesAuthSuite) assertJSONErrorResponse(c *gc.C, resp *http.Response, expCode int, expError string) {
	uploadResponse := s.assertResponse(c, resp, expCode)
	c.Check(uploadResponse.Error, gc.NotNil)
	c.Check(uploadResponse.Error.Message, gc.Matches, expError)
}

func (s *resourcesAuthSuite) assertPlainErrorResponse(c *gc.C, resp *http.Response, expCode int, expError string) {
	body := apitesting.AssertResponse(c, resp, expCode, "text/plain; charset=utf-8")
	c.Assert(string(body), gc.Matches, expError+"\n")
}

func (s *resourcesAuthSuite) assertResponse(c *gc.C, resp *http.Response, expStatus int) params.UploadResult {
	body := apitesting.AssertResponse(c, resp, expStatus, params.ContentTypeJSON)
	var uploadResult params.UploadResult
	err := json.Unmarshal(body, &uploadResult)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("Body: %s", body))
	return uploadResult
}

var _ = gc.Suite(&resourcesAuthSuite{})

func (s *resourcesAuthSuite) TestResourcesUploadedSecurely(c *gc.C) {
	url := s.resourcesURL("tomcat", "jdk")
	url.Scheme = "http"
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:       "PUT",
		URL:          url.String(),
		ExpectStatus: http.StatusBadRequest,
	})
	defer resp.Body.Close()
}

func (s *resourcesAuthSuite) TestRequiresAuth(c *gc.C) {
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: s.resourcesURL("tomcat", "jdk").String()})
	defer resp.Body.Close()
	s.assertPlainErrorResponse(c, resp, http.StatusUnauthorized, "authentication failed: no credentials provided")
}

func (s *resourcesAuthSuite) TestAuthRejectsNonsUser(c *gc.C) {
	// Add a machine and try to login.
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("foo", "", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)

	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Tag:      machine.Tag().String(),
		Password: password,
		Method:   "PUT",
		URL:      s.resourcesURL("tomcat", "jdk").String(),
		Nonce:    "fake_nonce",
	})
	s.assertPlainErrorResponse(
		c, resp, http.StatusForbidden,
		"authorization failed: permission denied",
	)
	resp.Body.Close()

	// Now try a user login.
	content, err := resources.GenerateContent(strings.NewReader("resource"))
	c.Assert(err, jc.ErrorIsNil)
	filename := mime.BEncoding.Encode("utf-8", "foo.txt")
	disp := mime.FormatMediaType(
		"form-data",
		map[string]string{"filename": filename},
	)

	resp = s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:      "PUT",
		URL:         s.resourcesURL("tomcat", "jdk").String(),
		ContentType: "application/octet-stream",
		ExtraHeaders: map[string]string{
			"Content-Sha384":      content.Fingerprint.String(),
			"Content-Length":      fmt.Sprintf("%d", content.Size),
			"Content-Disposition": disp,
		},
		Body: strings.NewReader("fake_nonce"),
	})
	s.assertJSONErrorResponse(c, resp, http.StatusNotFound, `application "tomcat" not found`)
	resp.Body.Close()
}

func (s *resourcesAuthSuite) TestUploadAuthRejectsUserWithoutPermission(c *gc.C) {
	s.Factory.MakeUser(c, &factory.UserParams{
		Name:     "oryx",
		Password: "gardener",
		Access:   permission.ReadAccess,
	})
	s.assertAuthRejectsUserWithoutPermission(c, "PUT")
}

func (s *resourcesAuthSuite) TestDownloadAuthRejectsUserWithoutPermission(c *gc.C) {
	s.Factory.MakeUser(c, &factory.UserParams{
		Name:        "oryx",
		Password:    "gardener",
		NoModelUser: true,
	})
	s.assertAuthRejectsUserWithoutPermission(c, "GET")
}

func (s *resourcesAuthSuite) assertAuthRejectsUserWithoutPermission(c *gc.C, method string) {

	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Tag:      "user-oryx",
		Password: "gardener",
		Method:   method,
		URL:      s.resourcesURL("tomcat", "jdk").String(),
	})
	defer resp.Body.Close()
	s.assertPlainErrorResponse(
		c, resp, http.StatusForbidden,
		"authorization failed: permission denied",
	)
}
