// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6/resource"

	"github.com/juju/juju/apiserver/params"
	apitesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/component/all"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type resourcesUploadSuite struct {
	apiserverBaseSuite
	appName        string
	unit           *state.Unit
	importingState *state.State
	importingModel *state.Model
}

var _ = gc.Suite(&resourcesUploadSuite{})

func (s *resourcesUploadSuite) SetUpSuite(c *gc.C) {
	s.apiserverBaseSuite.SetUpSuite(c)
	all.RegisterForServer()
}

func (s *resourcesUploadSuite) SetUpTest(c *gc.C) {
	s.apiserverBaseSuite.SetUpTest(c)

	// Create an importing model to work with.
	var err error
	s.importingState = s.Factory.MakeModel(c, nil)
	s.AddCleanup(func(*gc.C) { s.importingState.Close() })
	s.importingModel, err = s.importingState.Model()
	c.Assert(err, jc.ErrorIsNil)

	newFactory := factory.NewFactory(s.importingState)
	app := newFactory.MakeApplication(c, nil)
	s.appName = app.Name()

	s.unit = newFactory.MakeUnit(c, &factory.UnitParams{
		Application: app,
	})

	err = s.importingModel.SetMigrationMode(state.MigrationModeImporting)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *resourcesUploadSuite) sendHTTPRequest(c *gc.C, p apitesting.HTTPRequestParams) *http.Response {
	p.ExtraHeaders = map[string]string{
		params.MigrationModelHTTPHeader: s.importingModel.UUID(),
	}
	return s.apiserverBaseSuite.sendHTTPRequest(c, p)
}

func (s *resourcesUploadSuite) TestServedSecurely(c *gc.C) {
	url := s.resourcesURL("")
	url.Scheme = "http"
	apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:      "GET",
		URL:         url.String(),
		ExpectError: `.*malformed HTTP response.*`,
	})
}

func (s *resourcesUploadSuite) TestGETUnsupported(c *gc.C) {
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: s.resourcesURI("")})
	s.assertErrorResponse(c, resp, http.StatusMethodNotAllowed, `unsupported method: "GET"`)
}

func (s *resourcesUploadSuite) TestPUTUnsupported(c *gc.C) {
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "PUT", URL: s.resourcesURI("")})
	s.assertErrorResponse(c, resp, http.StatusMethodNotAllowed, `unsupported method: "PUT"`)
}

func (s *resourcesUploadSuite) TestPOSTRequiresAuth(c *gc.C) {
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "POST", URL: s.resourcesURI("")})
	body := apitesting.AssertResponse(c, resp, http.StatusUnauthorized, "text/plain; charset=utf-8")
	c.Assert(string(body), gc.Equals, "authentication failed: no credentials provided\n")
}

func (s *resourcesUploadSuite) TestPOSTRequiresUserAuth(c *gc.C) {
	// Add a machine and try to login.
	machine, password := s.Factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Nonce: "noncy",
	})
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Tag:         machine.Tag().String(),
		Password:    password,
		Method:      "POST",
		URL:         s.resourcesURI(""),
		Nonce:       "noncy",
		ContentType: "foo/bar",
	})
	body := apitesting.AssertResponse(c, resp, http.StatusForbidden, "text/plain; charset=utf-8")
	c.Assert(string(body), gc.Equals, "authorization failed: machine 0 is not a user\n")

	// Now try a user login.
	resp = s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "POST", URL: s.resourcesURI("")})
	s.assertErrorResponse(c, resp, http.StatusBadRequest, "missing application/unit")
}

func (s *resourcesUploadSuite) TestRejectsInvalidModel(c *gc.C) {
	params := apitesting.HTTPRequestParams{
		Method: "POST",
		URL:    s.resourcesURI(""),
		ExtraHeaders: map[string]string{
			params.MigrationModelHTTPHeader: "dead-beef-123456",
		},
	}
	resp := s.apiserverBaseSuite.sendHTTPRequest(c, params)
	s.assertErrorResponse(c, resp, http.StatusNotFound, `.*unknown model: "dead-beef-123456"`)
}

const content = "stuff"

func (s *resourcesUploadSuite) makeUploadArgs(c *gc.C) url.Values {
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
	q.Add("timestamp", fmt.Sprint(time.Now().UnixNano()))
	return q
}

func (s *resourcesUploadSuite) TestUpload(c *gc.C) {
	outResp := s.uploadAppResource(c, nil)
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

func (s *resourcesUploadSuite) TestUnitUpload(c *gc.C) {
	// Upload application resource first. A unit resource can't be
	// uploaded without the application resource being there first.
	s.uploadAppResource(c, nil)

	q := s.makeUploadArgs(c)
	q.Del("application")
	q.Set("unit", s.unit.Name())
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:      "POST",
		URL:         s.resourcesURI(q.Encode()),
		ContentType: "application/octet-stream",
		Body:        strings.NewReader(content),
	})
	outResp := s.assertResponse(c, resp, http.StatusOK)
	c.Check(outResp.ID, gc.Not(gc.Equals), "")
	c.Check(outResp.Timestamp.IsZero(), jc.IsFalse)
}

func (s *resourcesUploadSuite) TestPlaceholder(c *gc.C) {
	query := s.makeUploadArgs(c)
	query.Del("timestamp") // No timestamp means placeholder
	outResp := s.uploadAppResource(c, &query)
	c.Check(outResp.ID, gc.Not(gc.Equals), "")
	c.Check(outResp.Timestamp.IsZero(), jc.IsTrue)

	rSt, err := s.importingState.Resources()
	c.Assert(err, jc.ErrorIsNil)
	res, err := rSt.GetResource(s.appName, "bin")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(res.IsPlaceholder(), jc.IsTrue)
	c.Check(res.ApplicationID, gc.Equals, s.appName)
	c.Check(res.Name, gc.Equals, "bin")
	c.Check(res.Size, gc.Equals, int64(len(content)))
}

func (s *resourcesUploadSuite) uploadAppResource(c *gc.C, query *url.Values) params.ResourceUploadResult {
	if query == nil {
		q := s.makeUploadArgs(c)
		query = &q
	}
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:      "POST",
		URL:         s.resourcesURI(query.Encode()),
		ContentType: "application/octet-stream",
		Body:        strings.NewReader(content),
	})
	return s.assertResponse(c, resp, http.StatusOK)
}

func (s *resourcesUploadSuite) TestArgValidation(c *gc.C) {
	checkBadRequest := func(q url.Values, expected string) {
		resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
			Method: "POST",
			URL:    s.resourcesURI(q.Encode()),
		})
		s.assertErrorResponse(c, resp, http.StatusBadRequest, expected)
	}

	q := s.makeUploadArgs(c)
	q.Del("application")
	checkBadRequest(q, "missing application/unit")

	q = s.makeUploadArgs(c)
	q.Set("unit", "some/0")
	checkBadRequest(q, "application and unit can't be set at the same time")

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

func (s *resourcesUploadSuite) TestFailsWhenModelNotImporting(c *gc.C) {
	err := s.importingModel.SetMigrationMode(state.MigrationModeNone)
	c.Assert(err, jc.ErrorIsNil)

	q := s.makeUploadArgs(c)
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:      "POST",
		URL:         s.resourcesURI(q.Encode()),
		ContentType: "application/octet-stream",
		Body:        strings.NewReader(content),
	})
	s.assertResponse(c, resp, http.StatusBadRequest)
}

func (s *resourcesUploadSuite) resourcesURI(query string) string {
	if query != "" && query[0] == '?' {
		query = query[1:]
	}
	return s.resourcesURL(query).String()
}

func (s *resourcesUploadSuite) resourcesURL(query string) *url.URL {
	url := s.URL("/migrate/resources", nil)
	url.RawQuery = query
	return url
}

func (s *resourcesUploadSuite) assertErrorResponse(c *gc.C, resp *http.Response, expStatus int, expError string) {
	outResp := s.assertResponse(c, resp, expStatus)
	err := outResp.Error
	c.Assert(err, gc.NotNil)
	c.Check(err.Message, gc.Matches, expError)
}

func (s *resourcesUploadSuite) assertResponse(c *gc.C, resp *http.Response, expStatus int) params.ResourceUploadResult {
	body := apitesting.AssertResponse(c, resp, expStatus, params.ContentTypeJSON)
	var outResp params.ResourceUploadResult
	err := json.Unmarshal(body, &outResp)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("Body: %s", body))
	return outResp
}
