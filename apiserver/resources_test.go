// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/component/all"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type resourceUploadSuite struct {
	authHTTPSuite
	appName        string
	importingState *state.State
	importingModel *state.Model
}

var _ = gc.Suite(&resourceUploadSuite{})

func (s *resourceUploadSuite) SetUpSuite(c *gc.C) {
	s.authHTTPSuite.SetUpSuite(c)
	all.RegisterForServer()
}

func (s *resourceUploadSuite) SetUpTest(c *gc.C) {
	s.authHTTPSuite.SetUpTest(c)

	// Make the user a controller admin (required for migrations).
	controllerTag := names.NewControllerTag(s.ControllerConfig.ControllerUUID())
	_, err := s.State.SetUserAccess(s.userTag, controllerTag, permission.SuperuserAccess)
	c.Assert(err, jc.ErrorIsNil)

	// Create an importing model to work with.
	s.importingState = s.Factory.MakeModel(c, nil)
	s.AddCleanup(func(*gc.C) { s.importingState.Close() })
	s.importingModel, err = s.importingState.Model()
	c.Assert(err, jc.ErrorIsNil)

	newFactory := factory.NewFactory(s.importingState)
	app := newFactory.MakeApplication(c, nil)
	s.appName = app.Name()

	err = s.importingModel.SetMigrationMode(state.MigrationModeImporting)
	c.Assert(err, jc.ErrorIsNil)

	s.extraHeaders = map[string]string{
		params.MigrationModelHTTPHeader: s.importingModel.UUID(),
	}
}

func (s *resourceUploadSuite) TestServedSecurely(c *gc.C) {
	url := s.resourcesURL(c, "")
	url.Scheme = "http"
	s.sendRequest(c, httpRequestParams{
		method:      "GET",
		url:         url.String(),
		expectError: `.*malformed HTTP response.*`,
	})
}

func (s *resourceUploadSuite) TestGETUnsupported(c *gc.C) {
	resp := s.authRequest(c, httpRequestParams{method: "GET", url: s.resourcesURI(c, "")})
	s.assertErrorResponse(c, resp, http.StatusMethodNotAllowed, `unsupported method: "GET"`)
}

func (s *resourceUploadSuite) TestPUTUnsupported(c *gc.C) {
	resp := s.authRequest(c, httpRequestParams{method: "PUT", url: s.resourcesURI(c, "")})
	s.assertErrorResponse(c, resp, http.StatusMethodNotAllowed, `unsupported method: "PUT"`)
}

func (s *resourceUploadSuite) TestPOSTRequiresAuth(c *gc.C) {
	resp := s.sendRequest(c, httpRequestParams{method: "POST", url: s.resourcesURI(c, "")})
	s.assertErrorResponse(c, resp, http.StatusUnauthorized, ".*no credentials provided$")
}

func (s *resourceUploadSuite) TestPOSTRequiresUserAuth(c *gc.C) {
	// Add a machine and try to login.
	machine, password := s.Factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Nonce: "noncy",
	})
	resp := s.sendRequest(c, httpRequestParams{
		tag:         machine.Tag().String(),
		password:    password,
		method:      "POST",
		url:         s.resourcesURI(c, ""),
		nonce:       "noncy",
		contentType: "foo/bar",
	})
	s.assertErrorResponse(c, resp, http.StatusInternalServerError, ".*tag kind machine not valid$")

	// Now try a user login.
	resp = s.authRequest(c, httpRequestParams{method: "POST", url: s.resourcesURI(c, "")})
	s.assertErrorResponse(c, resp, http.StatusBadRequest, "missing application")
}

func (s *resourceUploadSuite) TestRejectsInvalidModel(c *gc.C) {
	s.extraHeaders[params.MigrationModelHTTPHeader] = "dead-beef-123456"
	resp := s.authRequest(c, httpRequestParams{method: "POST", url: s.resourcesURI(c, "")})
	s.assertErrorResponse(c, resp, http.StatusNotFound, `.*unknown model: "dead-beef-123456"$`)
}

const content = "stuff"

func (s *resourceUploadSuite) makeUploadArgs(c *gc.C) url.Values {
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)
	q := make(url.Values)
	q.Add("application", s.appName)
	q.Add("user", "napolean")
	q.Add("name", "bin")
	q.Add("path", "blob.zip")
	q.Add("description", "hmm")
	q.Add("type", "file")
	q.Add("origin", "store")
	q.Add("revision", "3")
	q.Add("size", fmt.Sprint(len(content)))
	q.Add("fingerprint", fp.Hex())
	return q
}

func (s *resourceUploadSuite) TestUpload(c *gc.C) {
	q := s.makeUploadArgs(c)
	resp := s.authRequest(c, httpRequestParams{
		method:      "POST",
		url:         s.resourcesURI(c, q.Encode()),
		contentType: "application/octet-stream",
		body:        strings.NewReader(content),
	})
	outResp := s.assertResponse(c, resp, http.StatusOK)
	c.Check(outResp.ID, gc.Not(gc.Equals), "")
	c.Check(outResp.Timestamp.IsZero(), jc.IsFalse)

	rSt, err := s.importingState.Resources()
	c.Assert(err, jc.ErrorIsNil)
	res, reader, err := rSt.OpenResource(s.appName, "bin")
	c.Assert(err, jc.ErrorIsNil)
	defer reader.Close()
	readContent, err := ioutil.ReadAll(reader)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(readContent), gc.Equals, content)
	c.Assert(res.ID, gc.Equals, outResp.ID)
}

func (s *resourceUploadSuite) TestArgValidation(c *gc.C) {
	checkBadRequest := func(q url.Values, expected string) {
		resp := s.authRequest(c, httpRequestParams{
			method: "POST",
			url:    s.resourcesURI(c, q.Encode()),
		})
		s.assertErrorResponse(c, resp, http.StatusBadRequest, expected)
	}

	q := s.makeUploadArgs(c)
	q.Del("application")
	checkBadRequest(q, "missing application")

	q = s.makeUploadArgs(c)
	q.Del("name")
	checkBadRequest(q, "missing name")

	q = s.makeUploadArgs(c)
	q.Del("path")
	checkBadRequest(q, "missing path")

	q = s.makeUploadArgs(c)
	q.Del("description")
	checkBadRequest(q, "missing description")

	q = s.makeUploadArgs(c)
	q.Set("type", "fooo")
	checkBadRequest(q, "invalid type")

	q = s.makeUploadArgs(c)
	q.Set("origin", "fooo")
	checkBadRequest(q, "invalid origin")

	q = s.makeUploadArgs(c)
	q.Set("revision", "fooo")
	checkBadRequest(q, "invalid revision")

	q = s.makeUploadArgs(c)
	q.Set("size", "fooo")
	checkBadRequest(q, "invalid size")

	q = s.makeUploadArgs(c)
	q.Set("fingerprint", "zzz")
	checkBadRequest(q, "invalid fingerprint")
}

func (s *resourceUploadSuite) TestFailsWhenModelNotImporting(c *gc.C) {
	err := s.importingModel.SetMigrationMode(state.MigrationModeNone)
	c.Assert(err, jc.ErrorIsNil)

	q := s.makeUploadArgs(c)
	resp := s.authRequest(c, httpRequestParams{
		method:      "POST",
		url:         s.resourcesURI(c, q.Encode()),
		contentType: "application/octet-stream",
		body:        strings.NewReader(content),
	})
	s.assertResponse(c, resp, http.StatusBadRequest)
}

func (s *resourceUploadSuite) resourcesURI(c *gc.C, query string) string {
	if query != "" && query[0] == '?' {
		query = query[1:]
	}
	return s.resourcesURL(c, query).String()
}

func (s *resourceUploadSuite) resourcesURL(c *gc.C, query string) *url.URL {
	uri := s.baseURL(c)
	uri.Path = "/migrate/resources"
	uri.RawQuery = query
	return uri
}

func (s *resourceUploadSuite) assertErrorResponse(c *gc.C, resp *http.Response, expStatus int, expError string) {
	outResp := s.assertResponse(c, resp, expStatus)
	err := outResp.Error
	c.Assert(err, gc.NotNil)
	c.Check(err.Message, gc.Matches, expError)
}

func (s *resourceUploadSuite) assertResponse(c *gc.C, resp *http.Response, expStatus int) params.ResourceUploadResult {
	body := assertResponse(c, resp, expStatus, params.ContentTypeJSON)
	var outResp params.ResourceUploadResult
	err := json.Unmarshal(body, &outResp)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("body: %s", body))
	return outResp
}
