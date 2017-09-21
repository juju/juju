// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"runtime"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
	jujuFactory "github.com/juju/juju/testing/factory"
)

// restCommonSuite wraps authHTTPSuite and adds
// some helper methods suitable for working with the
// charms endpoint.
type restCommonSuite struct {
	authHTTPSuite
}

func (s *restCommonSuite) restURL(c *gc.C, path string) *url.URL {
	uri := s.baseURL(c)
	uri.Path = fmt.Sprintf("/model/%s/rest/1.0/%s", s.modelUUID, path)
	return uri
}

func (s *restCommonSuite) restURI(c *gc.C, path string) string {
	return s.restURL(c, path).String()
}

func (s *restCommonSuite) assertGetFileResponse(c *gc.C, resp *http.Response, expBody, expContentType string) {
	body := assertResponse(c, resp, http.StatusOK, expContentType)
	c.Check(string(body), gc.Equals, expBody)
}

func (s *restCommonSuite) assertErrorResponse(c *gc.C, resp *http.Response, expCode int, expError string) {
	charmResponse := s.assertResponse(c, resp, expCode)
	c.Check(charmResponse.Error, gc.Matches, expError)
}

func (s *restCommonSuite) assertResponse(c *gc.C, resp *http.Response, expStatus int) params.CharmsResponse {
	body := assertResponse(c, resp, expStatus, params.ContentTypeJSON)
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
	info := s.APIInfo(c)
	uri := "http://" + info.Addrs[0] + "/rest"
	s.sendRequest(c, httpRequestParams{
		method:      "GET",
		url:         uri,
		expectError: `.*malformed HTTP response.*`,
	})
}

func (s *restSuite) TestGETRequiresAuth(c *gc.C) {
	resp := s.sendRequest(c, httpRequestParams{method: "GET", url: s.restURI(c, "entity/name/attribute")})
	s.assertErrorResponse(c, resp, http.StatusUnauthorized, ".*no credentials provided$")
}

func (s *restSuite) TestRequiresGET(c *gc.C) {
	resp := s.authRequest(c, httpRequestParams{method: "POST", url: s.restURI(c, "entity/name/attribute")})
	s.assertErrorResponse(c, resp, http.StatusMethodNotAllowed, `unsupported method: "POST"`)
}

func (s *restSuite) TestGetReturnsNotFoundWhenMissing(c *gc.C) {
	uri := s.restURI(c, "remote-application/foo/attribute")
	resp := s.authRequest(c, httpRequestParams{method: "GET", url: uri})
	s.assertErrorResponse(
		c, resp, http.StatusNotFound,
		`cannot retrieve model data: remote application "foo" not found`,
	)
}

func (s *restSuite) charmsURI(c *gc.C, query string) string {
	uri := s.baseURL(c)
	uri.Path = fmt.Sprintf("/model/%s/charms", s.modelUUID)
	uri.RawQuery = query
	return uri.String()
}

func (s *restSuite) TestGetRemoteApplicationIcon(c *gc.C) {
	// Setup the charm and mysql application in the default model.
	ch := testcharms.Repo.CharmArchive(c.MkDir(), "mysql")
	s.uploadRequest(c, s.charmsURI(c, "series=quantal"), "application/zip", ch.Path)
	curl, err := charm.ParseURL(fmt.Sprintf("local:quantal/%s-%d", ch.Meta().Name, ch.Revision()))
	c.Assert(err, jc.ErrorIsNil)
	mysqlCh, err := s.State.Charm(curl)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddApplication(state.AddApplicationArgs{
		Name:  "mysql",
		Type:  state.ApplicationTypeIAAS,
		Charm: mysqlCh,
	})
	c.Assert(err, jc.ErrorIsNil)
	// Add an offer for the application.
	offers := state.NewApplicationOffers(s.State)
	offer, err := offers.AddOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "remote-app-offer",
		ApplicationName: "mysql",
		Owner:           "admin",
	})
	c.Assert(err, jc.ErrorIsNil)
	// Set up a charm entry for dummy app with no charm in storage.
	factory := jujuFactory.NewFactory(s.State)
	dummyCh := factory.MakeCharm(c, &jujuFactory.CharmParams{
		Name: "dummy",
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddApplication(state.AddApplicationArgs{
		Name:  "dummy",
		Type:  state.ApplicationTypeIAAS,
		Charm: dummyCh,
	})
	c.Assert(err, jc.ErrorIsNil)
	offer2, err := offers.AddOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "notfound-remote-app-offer",
		ApplicationName: "dummy",
		Owner:           "admin",
	})
	c.Assert(err, jc.ErrorIsNil)

	// Add remote applications to other model which we will query below.
	otherModelState := s.setupOtherModel(c)
	_, err = otherModelState.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "remote-app",
		SourceModel: s.IAASModel.ModelTag(),
		OfferUUID:   offer.OfferUUID,
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = otherModelState.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "notfound-remote-app",
		SourceModel: s.IAASModel.ModelTag(),
		OfferUUID:   offer2.OfferUUID,
	})
	c.Assert(err, jc.ErrorIsNil)

	// Prepare the tests.
	svgMimeType := mime.TypeByExtension(".svg")
	iconPath := filepath.Join(testcharms.Repo.CharmDirPath("mysql"), "icon.svg")
	icon, err := ioutil.ReadFile(iconPath)
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
		uri := s.restURI(c, test.query)
		resp := s.authRequest(c, httpRequestParams{method: "GET", url: uri})
		if test.expectType == "" {
			test.expectType = svgMimeType
		}
		s.assertGetFileResponse(c, resp, test.expectBody, test.expectType)
	}
}

func (s *restSuite) TestGetRejectsWrongModelUUIDPath(c *gc.C) {
	url := s.restURL(c, "")
	url.Path = "/model/dead-beef-123456/rest/1.0/remote-application/foo/icon"
	resp := s.authRequest(c, httpRequestParams{method: "GET", url: url.String()})
	s.assertErrorResponse(c, resp, http.StatusNotFound, `.*unknown model: "dead-beef-123456"$`)
}

type restWithMacaroonsSuite struct {
	restCommonSuite
}

var _ = gc.Suite(&restWithMacaroonsSuite{})

func (s *restWithMacaroonsSuite) SetUpTest(c *gc.C) {
	s.macaroonAuthEnabled = true
	s.authHTTPSuite.SetUpTest(c)
}

func (s *restWithMacaroonsSuite) TestWithNoBasicAuthReturnsDischargeRequiredError(c *gc.C) {
	resp := s.sendRequest(c, httpRequestParams{
		method: "GET",
		url:    s.restURI(c, "remote-application/foo/icon"),
	})

	restResponse := s.assertResponse(c, resp, http.StatusUnauthorized)
	c.Assert(restResponse.Error, gc.Matches, ".*verification failed: no macaroons$")
	c.Assert(restResponse.ErrorCode, gc.Equals, params.CodeDischargeRequired)
	c.Assert(restResponse.ErrorInfo, gc.NotNil)
	c.Assert(restResponse.ErrorInfo.Macaroon, gc.NotNil)
}

func (s *restWithMacaroonsSuite) TestGetWithDischargedMacaroon(c *gc.C) {
	checkCount := 0
	s.DischargerLogin = func() string {
		checkCount++
		return s.userTag.Id()
	}
	resp := s.sendRequest(c, httpRequestParams{
		do:          s.doer(),
		method:      "GET",
		url:         s.restURI(c, "remote-application/foo/icon"),
		contentType: "foo/bar",
	})
	s.assertErrorResponse(c, resp, http.StatusNotFound, `cannot retrieve model data: remote application "foo" not found`)
	c.Assert(checkCount, gc.Equals, 1)
}

// doer returns a Do function that can make a bakery request
// appropriate for a charms endpoint.
func (s *restWithMacaroonsSuite) doer() func(*http.Request) (*http.Response, error) {
	return bakeryDo(nil, charmsBakeryGetError)
}
