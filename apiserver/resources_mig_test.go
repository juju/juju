// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"

	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

type resourcesUploadSuite struct {
	applicationsService *MockApplicationService
	resourceService     *MockResourceService

	stateAuthErr error

	mockCtrl *gomock.Controller
	srv      *httptest.Server
}

var _ = gc.Suite(&resourcesUploadSuite{})

func (s *resourcesUploadSuite) SetUpTest(c *gc.C) {
	s.mockCtrl = gomock.NewController(c)

	s.applicationsService = NewMockApplicationService(s.mockCtrl)
	applicationsServiceGetter := NewMockApplicationServiceGetter(s.mockCtrl)
	applicationsServiceGetter.EXPECT().Application(gomock.Any()).Return(s.applicationsService, nil).AnyTimes()

	s.resourceService = NewMockResourceService(s.mockCtrl)
	resourceServiceGetter := NewMockResourceServiceGetter(s.mockCtrl)
	resourceServiceGetter.EXPECT().Resource(gomock.Any()).Return(s.resourceService, nil).AnyTimes()

	handler := &resourcesMigrationUploadHandler{
		ctxt:                     httpContext{},
		stateAuthFunc:            s.authState,
		resourceServiceGetter:    resourceServiceGetter,
		applicationServiceGetter: applicationsServiceGetter,
	}
	s.srv = httptest.NewServer(handler)
}

func (s *resourcesUploadSuite) TearDownTest(c *gc.C) {
	s.srv.Close()
	s.mockCtrl.Finish()
}

type noopPoolState struct{}

func (p *noopPoolState) Release() bool {
	return true
}

func (s *resourcesUploadSuite) authState(req *http.Request) (poolStateHelper, error) {
	return &noopPoolState{}, errors.Capture(s.stateAuthErr)
}

func (s *resourcesUploadSuite) TestGETUnsupported(c *gc.C) {
	// Act
	resp, err := http.Get(s.srv.URL)
	c.Assert(err, gc.IsNil, gc.Commentf("(Act) Error executing request"))

	// Assert
	c.Assert(resp.StatusCode, gc.Equals, http.StatusMethodNotAllowed)
}

func (s *resourcesUploadSuite) TestPOSTRequiresAuth(c *gc.C) {
	// Arrange
	s.stateAuthErr = errors.New("auth error")

	// Act
	resp, err := http.Post(s.srv.URL, "", nil)
	c.Assert(err, gc.IsNil, gc.Commentf("(Act) Error executing request"))

	// Assert
	c.Assert(resp.StatusCode, gc.Equals, http.StatusUnauthorized)
}

func (s *resourcesUploadSuite) TestRejectsInvalidModel(c *gc.C) {
	// Arrange
	req, err := http.NewRequest("POST", s.srv.URL, nil)
	c.Assert(err, gc.IsNil, gc.Commentf("(Arrange) Error creating request"))

	req.Header.Set(params.MigrationModelHTTPHeader, "dead-beef-123456")

	// Act
	resp, err := http.DefaultClient.Do(req)
	c.Assert(err, gc.IsNil, gc.Commentf("(Act) Error executing request"))

	// Assert
	c.Assert(resp.StatusCode, gc.Equals, http.StatusNotFound,
		gc.Commentf("(Assert) unexpected response status"))
	s.decodeResponse(resp)
	c.Assert()

	s.assertErrorResponse(c, resp, http.StatusNotFound, `.*unknown model: "dead-beef-123456"`)
}

// const content = "stuff"
//
//	func (s *resourcesUploadSuite) makeUploadArgs(c *gc.C) url.Values {
//		return s.makeResourceUploadArgs(c, "file")
//	}
//
// //func (s *resourcesUploadSuite) makeDockerUploadArgs(c *gc.C) url.Values {
// //	result := s.makeResourceUploadArgs(c, "oci-image")
// //	result.Del("path")
// //	return result
// //}
//
// //func (s *resourcesUploadSuite) makeResourceUploadArgs(c *gc.C, resType string) url.Values {
// //	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
// //	c.Assert(err, jc.ErrorIsNil)
// //	q := make(url.Values)
// //	q.Add("application", s.appName)
// //	q.Add("user", "napoleon")
// //	q.Add("name", "bin")
// //	q.Add("path", "blob.zip")
// //	q.Add("description", "hmm")
// //	q.Add("type", resType)
// //	q.Add("origin", "store")
// //	q.Add("revision", "3")
// //	q.Add("size", fmt.Sprint(len(content)))
// //	q.Add("fingerprint", fp.Hex())
// //	q.Add("timestamp", fmt.Sprint(time.Now().UnixNano()))
// //	return q
// //}
//
// //func (s *resourcesUploadSuite) TestUpload(c *gc.C) {
// //	outResp := s.uploadAppResource(c, nil)
// //	c.Check(outResp.ID, gc.Not(gc.Equals), "")
// //	c.Check(outResp.Timestamp.IsZero(), jc.IsFalse)
// //
// //	store := s.ObjectStore(c, s.ControllerModelUUID())
// //	rSt := s.importingState.Resources(store)
// //	res, reader, err := rSt.OpenResource(s.appName, "bin")
// //	c.Assert(err, jc.ErrorIsNil)
// //	defer reader.Close()
// //	readContent, err := io.ReadAll(reader)
// //	c.Assert(err, jc.ErrorIsNil)
// //	c.Assert(string(readContent), gc.Equals, content)
// //	c.Assert(res.ID, gc.Equals, outResp.ID)
// //}
//
// //func (s *resourcesUploadSuite) TestUnitUpload(c *gc.C) {
// //	// Upload application resource first. A unit resource can't be
// //	// uploaded without the application resource being there first.
// //	s.uploadAppResource(c, nil)
// //
// //	q := s.makeUploadArgs(c)
// //	q.Del("application")
// //	q.Set("unit", s.unit.Name())
// //	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
// //		Method:      "POST",
// //		URL:         s.resourcesURI(q.Encode()),
// //		ContentType: "application/octet-stream",
// //		Body:        strings.NewReader(content),
// //	})
// //	outResp := s.assertResponse(c, resp, http.StatusOK)
// //	c.Check(outResp.ID, gc.Not(gc.Equals), "")
// //	c.Check(outResp.Timestamp.IsZero(), jc.IsFalse)
// //}
//
// //func (s *resourcesUploadSuite) TestPlaceholder(c *gc.C) {
// //	query := s.makeUploadArgs(c)
// //	query.Del("timestamp") // No timestamp means placeholder
// //	outResp := s.uploadAppResource(c, &query)
// //	c.Check(outResp.ID, gc.Not(gc.Equals), "")
// //	c.Check(outResp.Timestamp.IsZero(), jc.IsTrue)
// //
// //	store := testing.NewObjectStore(c, s.importingState.ModelUUID())
// //	rSt := s.importingState.Resources(store)
// //	res, err := rSt.GetResource(s.appName, "bin")
// //	c.Assert(err, jc.ErrorIsNil)
// //	c.Check(res.IsPlaceholder(), jc.IsTrue)
// //	c.Check(res.ApplicationID, gc.Equals, s.appName)
// //	c.Check(res.Name, gc.Equals, "bin")
// //	c.Check(res.Size, gc.Equals, int64(len(content)))
// //}
//
// //func (s *resourcesUploadSuite) uploadAppResource(c *gc.C, query *url.Values) params.ResourceUploadResult {
// //	if query == nil {
// //		q := s.makeUploadArgs(c)
// //		query = &q
// //	}
// //	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
// //		Method:      "POST",
// //		URL:         s.resourcesURI(query.Encode()),
// //		ContentType: "application/octet-stream",
// //		Body:        strings.NewReader(content),
// //	})
// //	return s.assertResponse(c, resp, http.StatusOK)
// //}
//
// //func (s *resourcesUploadSuite) TestArgValidation(c *gc.C) {
// //	checkBadRequest := func(q url.Values, expected string) {
// //		resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
// //			Method: "POST",
// //			URL:    s.resourcesURI(q.Encode()),
// //		})
// //		s.assertErrorResponse(c, resp, http.StatusBadRequest, expected)
// //	}
// //
// //	q := s.makeUploadArgs(c)
// //	q.Del("application")
// //	checkBadRequest(q, "missing application/unit")
// //
// //	q = s.makeUploadArgs(c)
// //	q.Set("unit", "some/0")
// //	checkBadRequest(q, "application and unit can't be set at the same time")
// //
// //	q = s.makeUploadArgs(c)
// //	q.Del("name")
// //	checkBadRequest(q, "missing name")
// //
// //	q = s.makeUploadArgs(c)
// //	q.Del("path")
// //	checkBadRequest(q, "missing path")
// //
// //	q = s.makeUploadArgs(c)
// //	q.Set("type", "fooo")
// //	checkBadRequest(q, "invalid type")
// //
// //	q = s.makeUploadArgs(c)
// //	q.Set("origin", "fooo")
// //	checkBadRequest(q, "invalid origin")
// //
// //	q = s.makeUploadArgs(c)
// //	q.Set("revision", "fooo")
// //	checkBadRequest(q, "invalid revision")
// //
// //	q = s.makeUploadArgs(c)
// //	q.Set("size", "fooo")
// //	checkBadRequest(q, "invalid size")
// //
// //	q = s.makeUploadArgs(c)
// //	q.Set("fingerprint", "zzz")
// //	checkBadRequest(q, "invalid fingerprint")
// //}
//
// //func (s *resourcesUploadSuite) TestArgValidationCAASModel(c *gc.C) {
// //	content := `{"ImageName": "image-name", "Username": "fred", "Password":"secret"}`
// //	checkRequest := func(q url.Values) {
// //		resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
// //			Method: "POST",
// //			URL:    s.resourcesURI(q.Encode()),
// //			Body:   bytes.NewReader([]byte(content)),
// //		})
// //		s.assertResponse(c, resp, http.StatusOK)
// //	}
// //
// //	q := s.makeDockerUploadArgs(c)
// //	checkRequest(q)
// //}
//
// //func (s *resourcesUploadSuite) TestFailsWhenModelNotImporting(c *gc.C) {
// //	err := s.importingModel.SetMigrationMode(state.MigrationModeNone)
// //	c.Assert(err, jc.ErrorIsNil)
// //
// //	q := s.makeUploadArgs(c)
// //	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
// //		Method:      "POST",
// //		URL:         s.resourcesURI(q.Encode()),
// //		ContentType: "application/octet-stream",
// //		Body:        strings.NewReader(content),
// //	})
// //	s.assertResponse(c, resp, http.StatusBadRequest)
// //}
//
//	func (s *resourcesUploadSuite) resourcesURI(query string) string {
//		if query != "" && query[0] == '?' {
//			query = query[1:]
//		}
//		return s.resourcesURL(query).String()
//	}
//
//	func (s *resourcesUploadSuite) resourcesURL(query string) *url.URL {
//		url := s.URL("/migrate/resources", nil)
//		url.RawQuery = query
//		return url
//	}

func (s *resourcesUploadSuite) decodeResponse(resp *http.Response) (params.ResourceUploadResult, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return params.ResourceUploadResult{}, errors.Capture(err)
	}
	_ = resp.Body.Close()

	ctype := resp.Header.Get("Content-Type")
	if ctype == params.ContentTypeJSON {
		return params.ResourceUploadResult{},
			errors.Errorf("unexpected content-type (expect %q, get %q",
				params.ContentTypeJSON, ctype)
	}

	var outResp params.ResourceUploadResult
	if err := json.Unmarshal(body, &outResp); err != nil {
		return params.ResourceUploadResult{}, errors.Capture(err)
	}
	return outResp, nil
}

//
//func (s *resourcesUploadSuite) TestSetResource(c *gc.C) {
//	ctrl := gomock.NewController(c)
//	defer ctrl.Finish()
//
//	stResources := mocks.NewMockResources(ctrl)
//	gomock.InOrder(
//		stResources.EXPECT().SetUnitResource(gomock.Any(), gomock.Any(), gomock.Any()).Return(resource.Resource{}, nil),
//		stResources.EXPECT().SetResource(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), state.DoNotIncrementCharmModifiedVersion).Return(resource.Resource{}, nil),
//	)
//	//apiserver.SetResource(true, "", "", charmresource.Resource{}, nil, stResources)
//	//apiserver.SetResource(false, "", "", charmresource.Resource{}, nil, stResources)
//}
