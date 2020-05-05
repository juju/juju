// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"time"

	charmresource "github.com/juju/charm/v7/resource"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api"
	"github.com/juju/juju/resource/resourcetesting"
	"github.com/juju/juju/state"
)

type ResourcesHandlerSuite struct {
	testing.IsolationSuite

	stateAuthErr error
	backend      *fakeBackend
	username     string
	req          *http.Request
	header       http.Header
	recorder     *httptest.ResponseRecorder
	handler      *apiserver.ResourcesHandler
}

var _ = gc.Suite(&ResourcesHandlerSuite{})

func (s *ResourcesHandlerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stateAuthErr = nil
	s.backend = new(fakeBackend)
	s.username = "youknowwho"

	method := "..."
	urlStr := "..."
	body := strings.NewReader("...")
	req, err := http.NewRequest(method, urlStr, body)
	c.Assert(err, jc.ErrorIsNil)
	s.req = req
	s.recorder = httptest.NewRecorder()
	s.handler = &apiserver.ResourcesHandler{
		StateAuthFunc:     s.authState,
		ChangeAllowedFunc: func(*http.Request) error { return nil },
	}
}

func (s *ResourcesHandlerSuite) authState(req *http.Request, tagKinds ...string) (
	apiserver.ResourcesBackend, state.PoolHelper, names.Tag, error,
) {
	if s.stateAuthErr != nil {
		return nil, nil, nil, errors.Trace(s.stateAuthErr)
	}

	ph := apiservertesting.StubPoolHelper{StubRelease: func() bool { return false }}
	tag := names.NewUserTag(s.username)
	return s.backend, ph, tag, nil
}

func (s *ResourcesHandlerSuite) TestExpectedAuthTags(c *gc.C) {
	expectedTags := set.NewStrings(names.UserTagKind, names.MachineTagKind, names.ControllerAgentTagKind, names.ApplicationTagKind)

	s.handler.StateAuthFunc = func(req *http.Request, tagKinds ...string) (apiserver.ResourcesBackend, state.PoolHelper, names.Tag, error) {
		gotTags := set.NewStrings(tagKinds...)
		if gotTags.Difference(expectedTags).Size() != 0 || expectedTags.Difference(gotTags).Size() != 0 {
			c.Fatalf("unexpected tag kinds %v", tagKinds)
			return nil, nil, nil, errors.NotValidf("tag kinds %v", tagKinds)
		}
		ph := apiservertesting.StubPoolHelper{StubRelease: func() bool { return false }}
		tag := names.NewUserTag(s.username)
		return s.backend, ph, tag, nil
	}
	s.req.Method = "GET"
	s.handler.ServeHTTP(s.recorder, s.req)
	s.checkResp(c, http.StatusOK, "application/octet-stream", resourceBody)
}

func (s *ResourcesHandlerSuite) TestStateAuthFailure(c *gc.C) {
	failure, expected := apiFailure("<failure>", "")
	s.stateAuthErr = failure

	s.handler.ServeHTTP(s.recorder, s.req)

	s.checkResp(c, http.StatusInternalServerError, "application/json", expected)
}

func (s *ResourcesHandlerSuite) TestUnsupportedMethod(c *gc.C) {
	s.req.Method = "POST"

	s.handler.ServeHTTP(s.recorder, s.req)

	_, expected := apiFailure(`unsupported method: "POST"`, params.CodeMethodNotAllowed)
	s.checkResp(c, http.StatusMethodNotAllowed, "application/json", expected)
}

func (s *ResourcesHandlerSuite) TestGetSuccess(c *gc.C) {
	s.req.Method = "GET"
	s.handler.ServeHTTP(s.recorder, s.req)
	s.checkResp(c, http.StatusOK, "application/octet-stream", resourceBody)
}

func (s *ResourcesHandlerSuite) TestPutSuccess(c *gc.C) {
	uploadContent := "<some data>"
	res, _ := newResource(c, "spam", "a-user", content)
	stored, _ := newResource(c, "spam", "", "")
	s.backend.ReturnGetResource = stored
	s.backend.ReturnSetResource = res

	req, _ := newUploadRequest(c, "spam", "a-application", uploadContent)
	s.handler.ServeHTTP(s.recorder, req)

	expected := mustMarshalJSON(&params.UploadResult{
		Resource: api.Resource2API(res),
	})
	s.checkResp(c, http.StatusOK, "application/json", string(expected))
}

func (s *ResourcesHandlerSuite) TestPutChangeBlocked(c *gc.C) {
	uploadContent := "<some data>"
	res, _ := newResource(c, "spam", "a-user", content)
	stored, _ := newResource(c, "spam", "", "")
	s.backend.ReturnGetResource = stored
	s.backend.ReturnSetResource = res

	expectedError := common.OperationBlockedError("test block")
	s.handler.ChangeAllowedFunc = func(*http.Request) error {
		return expectedError
	}

	req, _ := newUploadRequest(c, "spam", "a-application", uploadContent)
	s.handler.ServeHTTP(s.recorder, req)

	expected := mustMarshalJSON(&params.ErrorResult{common.ServerError(expectedError)})
	s.checkResp(c, http.StatusBadRequest, "application/json", string(expected))
}

func (s *ResourcesHandlerSuite) TestPutSuccessDockerResource(c *gc.C) {
	uploadContent := "<some data>"
	res := newDockerResource(c, "spam", "a-user", content)
	stored := newDockerResource(c, "spam", "", "")
	s.backend.ReturnGetResource = stored
	s.backend.ReturnSetResource = res

	req, _ := newUploadRequest(c, "spam", "a-application", uploadContent)
	s.handler.ServeHTTP(s.recorder, req)

	expected := mustMarshalJSON(&params.UploadResult{
		Resource: api.Resource2API(res),
	})
	s.checkResp(c, http.StatusOK, "application/json", string(expected))
}

func (s *ResourcesHandlerSuite) TestPutExtensionMismatch(c *gc.C) {
	content := "<some data>"

	// newResource returns a resource with a Path = name + ".tgz"
	res, _ := newResource(c, "spam", "a-user", content)
	stored, _ := newResource(c, "spam", "", "")
	s.backend.ReturnGetResource = stored
	s.backend.ReturnSetResource = res

	req, _ := newUploadRequest(c, "spam", "a-application", content)
	req.Header.Set("Content-Disposition", "form-data; filename=different.ext")
	s.handler.ServeHTTP(s.recorder, req)

	_, expected := apiFailure(`incorrect extension on resource upload "different.ext", expected ".tgz"`,
		"")
	s.checkResp(c, http.StatusInternalServerError, "application/json", expected)
}

func (s *ResourcesHandlerSuite) TestPutWithPending(c *gc.C) {
	uploadContent := "<some data>"
	res, _ := newResource(c, "spam", "a-user", uploadContent)
	res.PendingID = "some-unique-id"
	stored, _ := newResource(c, "spam", "", "")
	stored.PendingID = "some-unique-id"
	s.backend.ReturnGetPendingResource = stored
	s.backend.ReturnUpdatePendingResource = res

	req, _ := newUploadRequest(c, "spam", "a-application", content)
	req.URL.RawQuery += "&pendingid=some-unique-id"
	s.handler.ServeHTTP(s.recorder, req)

	expected := mustMarshalJSON(&params.UploadResult{
		Resource: api.Resource2API(res),
	})
	s.checkResp(c, http.StatusOK, "application/json", string(expected))
}

func (s *ResourcesHandlerSuite) TestPutSetResourceFailure(c *gc.C) {
	content := "<some data>"
	stored, _ := newResource(c, "spam", "", "")
	s.backend.ReturnGetResource = stored
	failure, expected := apiFailure("boom", "")
	s.backend.SetResourceErr = failure

	req, _ := newUploadRequest(c, "spam", "a-application", content)
	s.handler.ServeHTTP(s.recorder, req)
	s.checkResp(c, http.StatusInternalServerError, "application/json", expected)
}

func (s *ResourcesHandlerSuite) checkResp(c *gc.C, status int, ctype, body string) {
	checkHTTPResp(c, s.recorder, status, ctype, body)
}

func checkHTTPResp(c *gc.C, recorder *httptest.ResponseRecorder, status int, ctype, body string) {
	c.Assert(recorder.Code, gc.Equals, status)
	hdr := recorder.Header()
	c.Check(hdr.Get("Content-Type"), gc.Equals, ctype)
	c.Check(hdr.Get("Content-Length"), gc.Equals, strconv.Itoa(len(body)))

	actualBody, err := ioutil.ReadAll(recorder.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(actualBody), gc.Equals, body)
}

type fakeBackend struct {
	ReturnGetResource           resource.Resource
	ReturnGetPendingResource    resource.Resource
	ReturnSetResource           resource.Resource
	SetResourceErr              error
	ReturnUpdatePendingResource resource.Resource
}

const resourceBody = "body"

func (s *fakeBackend) OpenResource(application, name string) (resource.Resource, io.ReadCloser, error) {
	res := resource.Resource{}
	res.Size = int64(len(resourceBody))
	reader := ioutil.NopCloser(strings.NewReader(resourceBody))
	return res, reader, nil
}

func (s *fakeBackend) GetResource(service, name string) (resource.Resource, error) {
	return s.ReturnGetResource, nil
}

func (s *fakeBackend) GetPendingResource(service, name, pendingID string) (resource.Resource, error) {
	return s.ReturnGetPendingResource, nil
}

func (s *fakeBackend) SetResource(applicationID, userID string, res charmresource.Resource, r io.Reader) (resource.Resource, error) {
	if s.SetResourceErr != nil {
		return resource.Resource{}, s.SetResourceErr
	}
	return s.ReturnSetResource, nil
}

func (s *fakeBackend) UpdatePendingResource(applicationID, pendingID, userID string, res charmresource.Resource, r io.Reader) (resource.Resource, error) {
	return s.ReturnUpdatePendingResource, nil
}

func newDockerResource(c *gc.C, name, username, data string) resource.Resource {
	opened := resourcetesting.NewDockerResource(c, nil, name, "a-application", data)
	res := opened.Resource
	res.Username = username
	if username == "" {
		res.Timestamp = time.Time{}
	}
	return res
}

func newResource(c *gc.C, name, username, data string) (resource.Resource, params.Resource) {
	opened := resourcetesting.NewResource(c, nil, name, "a-application", data)
	res := opened.Resource
	res.Username = username
	if username == "" {
		res.Timestamp = time.Time{}
	}

	apiRes := params.Resource{
		CharmResource: params.CharmResource{
			Name:        name,
			Description: name + " description",
			Type:        "file",
			Path:        res.Path,
			Origin:      "upload",
			Revision:    0,
			Fingerprint: res.Fingerprint.Bytes(),
			Size:        res.Size,
		},
		ID:            res.ID,
		ApplicationID: res.ApplicationID,
		Username:      username,
		Timestamp:     res.Timestamp,
	}

	return res, apiRes
}

func newUploadRequest(c *gc.C, name, service, content string) (*http.Request, io.Reader) {
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)

	method := "PUT"
	urlStr := "https://api:17017/applications/%s/resources/%s"
	urlStr += "?:application=%s&:resource=%s" // ...added by the mux.
	urlStr = fmt.Sprintf(urlStr, service, name, service, name)
	body := strings.NewReader(content)
	req, err := http.NewRequest(method, urlStr, body)
	c.Assert(err, jc.ErrorIsNil)

	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Length", fmt.Sprint(len(content)))
	req.Header.Set("Content-SHA384", fp.String())
	req.Header.Set("Content-Disposition", "form-data; filename="+name+".tgz")

	return req, body
}

func apiFailure(msg, code string) (error, string) {
	failure := errors.New(msg)
	data := mustMarshalJSON(params.ErrorResult{
		Error: &params.Error{
			Message: msg,
			Code:    code,
		},
	})
	return failure, string(data)
}

func mustMarshalJSON(v interface{}) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}
