// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"

	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	apitesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/crossmodel"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/testing/factory"
)

type restCommonSuite struct {
	jujutesting.ApiServerSuite
}

func (s *restCommonSuite) restURL(modelUUID, path string) *url.URL {
	return s.URL(fmt.Sprintf("/model/%s/rest/1.0/%s", modelUUID, path), nil)
}

func (s *restCommonSuite) restURI(modelUUID, path string) string {
	return s.restURL(modelUUID, path).String()
}

func (s *restCommonSuite) assertGetFileResponse(c *gc.C, resp *http.Response, expBody, expContentType string) {
	body := apitesting.AssertResponse(c, resp, http.StatusOK, expContentType)
	c.Check(string(body), gc.Equals, expBody)
}

func (s *restCommonSuite) assertErrorResponse(c *gc.C, resp *http.Response, expCode int, expError string) {
	charmResponse := s.assertResponse(c, resp, expCode)
	c.Check(charmResponse.Error, gc.Matches, expError)
}

func (s *restCommonSuite) assertResponse(c *gc.C, resp *http.Response, expStatus int) params.CharmsResponse {
	body := apitesting.AssertResponse(c, resp, expStatus, params.ContentTypeJSON)
	var charmResponse params.CharmsResponse
	err := json.Unmarshal(body, &charmResponse)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("body: %s", body))
	return charmResponse
}

type restSuite struct {
	restCommonSuite
}

var _ = gc.Suite(&restSuite{})

func (s *restSuite) SetUpSuite(c *gc.C) {
	if runtime.GOOS != "linux" {
		c.Skip("apiservers only run on linux")
	}
	s.restCommonSuite.SetUpSuite(c)
}

func (s *restSuite) TestRestServedSecurely(c *gc.C) {
	url := s.restURL(s.ControllerModelUUID(), "")
	url.Scheme = "http"
	apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:       "GET",
		URL:          url.String(),
		ExpectStatus: http.StatusBadRequest,
	})
}

func (s *restSuite) TestGETRequiresAuth(c *gc.C) {
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: s.restURI(s.ControllerModelUUID(), "entity/name/attribute")})
	body := apitesting.AssertResponse(c, resp, http.StatusUnauthorized, "text/plain; charset=utf-8")
	c.Assert(string(body), gc.Equals, "authentication failed: no credentials provided\n")
}

func (s *restSuite) TestRequiresGET(c *gc.C) {
	resp := sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "POST", URL: s.restURI(s.ControllerModelUUID(), "entity/name/attribute")})
	s.assertErrorResponse(c, resp, http.StatusMethodNotAllowed, `unsupported method: "POST"`)
}

func (s *restSuite) TestGetReturnsNotFoundWhenMissing(c *gc.C) {
	uri := s.restURI(s.ControllerModelUUID(), "remote-application/foo/attribute")
	resp := sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: uri})
	s.assertErrorResponse(
		c, resp, http.StatusNotFound,
		`cannot retrieve model data: saas application "foo" not found`,
	)
}

func (s *restSuite) charmsURI(query string) string {
	url := s.URL(fmt.Sprintf("/model/%s/charms", s.ControllerModelUUID()), nil)
	url.RawQuery = query
	return url.String()
}

func (s *restSuite) TestGetRemoteApplicationIcon(c *gc.C) {
	// Setup the charm and mysql application in the default model.
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "mysql")

	file, err := os.Open(ch.Path)
	c.Assert(err, jc.ErrorIsNil)
	defer file.Close()
	resp := sendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:      "POST",
		URL:         s.charmsURI("series=quantal"),
		ContentType: "application/zip",
		Body:        file,
	})
	apitesting.AssertResponse(c, resp, http.StatusOK, "application/json")

	curl := fmt.Sprintf("local:quantal/%s-%d", ch.Meta().Name, ch.Revision())
	mysqlCh, err := s.ControllerModel(c).State().Charm(curl)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.ControllerModel(c).State().AddApplication(state.AddApplicationArgs{
		Name:        "mysql",
		Charm:       mysqlCh,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{OS: "ubuntu", Channel: "22.04/stable"}},
	})
	c.Assert(err, jc.ErrorIsNil)

	// Add an offer for the application.
	offers := state.NewApplicationOffers(s.ControllerModel(c).State())
	offer, err := offers.AddOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "remote-app-offer",
		ApplicationName: "mysql",
		Owner:           "admin",
	})
	c.Assert(err, jc.ErrorIsNil)
	// Set up a charm entry for dummy app with no charm in storage.
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	dummyCh := f.MakeCharm(c, &factory.CharmParams{
		Name: "dummy",
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.ControllerModel(c).State().AddApplication(state.AddApplicationArgs{
		Name:        "dummy",
		Charm:       dummyCh,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{OS: "ubuntu", Channel: "22.04/stable"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	offer2, err := offers.AddOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "notfound-remote-app-offer",
		ApplicationName: "dummy",
		Owner:           "admin",
	})
	c.Assert(err, jc.ErrorIsNil)

	// Add remote applications to other model which we will query below.
	otherModelState := f.MakeModel(c, nil)
	defer otherModelState.Close()
	_, err = otherModelState.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "remote-app",
		SourceModel: names.NewModelTag(s.ControllerModelUUID()),
		OfferUUID:   offer.OfferUUID,
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = otherModelState.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "notfound-remote-app",
		SourceModel: names.NewModelTag(s.ControllerModelUUID()),
		OfferUUID:   offer2.OfferUUID,
	})
	c.Assert(err, jc.ErrorIsNil)

	// Prepare the tests.
	svgMimeType := mime.TypeByExtension(".svg")
	iconPath := filepath.Join(testcharms.Repo.CharmDirPath("mysql"), "icon.svg")
	icon, err := os.ReadFile(iconPath)
	c.Assert(err, jc.ErrorIsNil)
	tests := []struct {
		about      string
		query      string
		expectType string
		expectBody string
	}{{
		about:      "icon found",
		query:      "remote-application/remote-app/icon",
		expectBody: string(icon),
	}, {
		about:      "icon not found",
		query:      "remote-application/notfound-remote-app/icon",
		expectBody: common.DefaultCharmIcon,
	}}

	for i, test := range tests {
		c.Logf("\ntest %d: %s", i, test.about)
		uri := s.restURI(otherModelState.ModelUUID(), test.query)
		resp := sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: uri})
		if test.expectType == "" {
			test.expectType = svgMimeType
		}
		s.assertGetFileResponse(c, resp, test.expectBody, test.expectType)
	}
}
