// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server_test

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource/api"
	"github.com/juju/juju/resource/api/server"
)

type UploadSuite struct {
	BaseSuite

	req    *http.Request
	header http.Header
	resp   *stubHTTPResponseWriter
}

var _ = gc.Suite(&UploadSuite{})

func (s *UploadSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	method := "..."
	urlStr := "..."
	body := strings.NewReader("...")
	req, err := http.NewRequest(method, urlStr, body)
	c.Assert(err, jc.ErrorIsNil)

	s.req = req
	s.header = make(http.Header)
	s.resp = &stubHTTPResponseWriter{
		stub:         s.stub,
		returnHeader: s.header,
	}
}

func (s *UploadSuite) TestHandleRequestOkay(c *gc.C) {
	content := "<some data>"
	res, _ := newResource(c, "spam", "a-user", content)
	stored, _ := newResource(c, "spam", "", "")
	s.data.ReturnGetResource = stored
	s.data.ReturnSetResource = res
	uh := server.UploadHandler{
		Username: "a-user",
		Store:    s.data,
	}
	req, body := newUploadRequest(c, "spam", "a-service", content)

	result, err := uh.HandleRequest(req)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "GetResource", "SetResource")
	s.stub.CheckCall(c, 0, "GetResource", "a-service", "spam")
	s.stub.CheckCall(c, 1, "SetResource", "a-service", "a-user", res.Resource, ioutil.NopCloser(body))
	c.Check(result, jc.DeepEquals, &api.UploadResult{
		Resource: api.Resource2API(res),
	})
}

func (s *UploadSuite) TestHandleRequestPending(c *gc.C) {
	content := "<some data>"
	res, _ := newResource(c, "spam", "a-user", content)
	res.PendingID = "some-unique-id"
	stored, _ := newResource(c, "spam", "", "")
	stored.PendingID = "some-unique-id"
	s.data.ReturnGetPendingResource = stored
	s.data.ReturnUpdatePendingResource = res
	uh := server.UploadHandler{
		Username: "a-user",
		Store:    s.data,
	}
	req, body := newUploadRequest(c, "spam", "a-service", content)
	req.URL.RawQuery += "&pendingid=some-unique-id"

	result, err := uh.HandleRequest(req)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "GetPendingResource", "UpdatePendingResource")
	s.stub.CheckCall(c, 0, "GetPendingResource", "a-service", "spam", "some-unique-id")
	s.stub.CheckCall(c, 1, "UpdatePendingResource", "a-service", "some-unique-id", "a-user", res.Resource, ioutil.NopCloser(body))
	c.Check(result, jc.DeepEquals, &api.UploadResult{
		Resource: api.Resource2API(res),
	})
}

func (s *UploadSuite) TestHandleRequestSetResourceFailure(c *gc.C) {
	content := "<some data>"
	stored, _ := newResource(c, "spam", "", "")
	s.data.ReturnGetResource = stored
	uh := server.UploadHandler{
		Username: "a-user",
		Store:    s.data,
	}
	req, _ := newUploadRequest(c, "spam", "a-service", content)
	failure := errors.New("<failure>")
	s.stub.SetErrors(nil, failure)

	_, err := uh.HandleRequest(req)

	c.Check(errors.Cause(err), gc.Equals, failure)
	s.stub.CheckCallNames(c, "GetResource", "SetResource")
}

func (s *UploadSuite) TestReadResourceOkay(c *gc.C) {
	content := "<some data>"
	expected, _ := newResource(c, "spam", "a-user", content)
	stored, _ := newResource(c, "spam", "", "")
	s.data.ReturnGetResource = stored
	uh := server.UploadHandler{
		Username: "a-user",
		Store:    s.data,
	}
	req, body := newUploadRequest(c, "spam", "a-service", content)

	uploaded, err := uh.ReadResource(req)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "GetResource")
	s.stub.CheckCall(c, 0, "GetResource", "a-service", "spam")
	c.Check(uploaded, jc.DeepEquals, &server.UploadedResource{
		Service:  "a-service",
		Resource: expected.Resource,
		Data:     ioutil.NopCloser(body),
	})
}

func (s *UploadSuite) TestReadResourcePending(c *gc.C) {
	content := "<some data>"
	expected, _ := newResource(c, "spam", "a-user", content)
	stored, _ := newResource(c, "spam", "", "")
	s.data.ReturnGetPendingResource = stored
	uh := server.UploadHandler{
		Username: "a-user",
		Store:    s.data,
	}
	req, body := newUploadRequest(c, "spam", "a-service", content)
	req.URL.RawQuery += "&pendingid=some-unique-id"

	uploaded, err := uh.ReadResource(req)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "GetPendingResource")
	s.stub.CheckCall(c, 0, "GetPendingResource", "a-service", "spam", "some-unique-id")
	c.Check(uploaded, jc.DeepEquals, &server.UploadedResource{
		Service:   "a-service",
		PendingID: "some-unique-id",
		Resource:  expected.Resource,
		Data:      ioutil.NopCloser(body),
	})
}

func (s *UploadSuite) TestReadResourceBadContentType(c *gc.C) {
	uh := server.UploadHandler{
		Username: "a-user",
		Store:    s.data,
	}
	req, _ := newUploadRequest(c, "spam", "a-service", "<some data>")
	req.Header.Set("Content-Type", "text/plain")

	_, err := uh.ReadResource(req)

	c.Check(err, gc.ErrorMatches, "unsupported content type .*")
	s.stub.CheckNoCalls(c)
}

func (s *UploadSuite) TestReadResourceGetResourceFailure(c *gc.C) {
	uh := server.UploadHandler{
		Username: "a-user",
		Store:    s.data,
	}
	req, _ := newUploadRequest(c, "spam", "a-service", "<some data>")
	failure := errors.New("<failure>")
	s.stub.SetErrors(failure)

	_, err := uh.ReadResource(req)

	c.Check(errors.Cause(err), gc.Equals, failure)
	s.stub.CheckCallNames(c, "GetResource")
}

func (s *UploadSuite) TestReadResourceBadFingerprint(c *gc.C) {
	stored, _ := newResource(c, "spam", "", "")
	s.data.ReturnGetResource = stored
	uh := server.UploadHandler{
		Username: "a-user",
		Store:    s.data,
	}
	req, _ := newUploadRequest(c, "spam", "a-service", "<some data>")
	req.Header.Set("Content-SHA384", "bogus")

	_, err := uh.ReadResource(req)

	c.Check(err, gc.ErrorMatches, "invalid fingerprint.*")
	s.stub.CheckNoCalls(c)
}

func (s *UploadSuite) TestReadResourceBadSize(c *gc.C) {
	stored, _ := newResource(c, "spam", "", "")
	s.data.ReturnGetResource = stored
	uh := server.UploadHandler{
		Username: "a-user",
		Store:    s.data,
	}
	req, _ := newUploadRequest(c, "spam", "a-service", "<some data>")
	req.Header.Set("Content-Length", "should-be-an-int")

	_, err := uh.ReadResource(req)

	c.Check(err, gc.ErrorMatches, "invalid size.*")
	s.stub.CheckNoCalls(c)
}

func newUploadRequest(c *gc.C, name, service, content string) (*http.Request, io.Reader) {
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)

	method := "PUT"
	urlStr := "https://api:17017/services/%s/resources/%s"
	urlStr += "?:service=%s&:resource=%s" // ...added by the mux.
	urlStr = fmt.Sprintf(urlStr, service, name, service, name)
	body := strings.NewReader(content)
	req, err := http.NewRequest(method, urlStr, body)
	c.Assert(err, jc.ErrorIsNil)

	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Length", fmt.Sprint(len(content)))
	req.Header.Set("Content-SHA384", fp.String())

	return req, body
}
