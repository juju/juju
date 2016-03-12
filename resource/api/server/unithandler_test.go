// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server_test

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/testing/filetesting"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api"
	"github.com/juju/juju/resource/api/server"
	"github.com/juju/juju/resource/resourcetesting"
)

var _ = gc.Suite(&UnitResourceHandlerSuite{})

type UnitResourceHandlerSuite struct {
	testing.IsolationSuite

	stub   *testing.Stub
	opener *stubResourceOpener
	deps   *stubUnitResourceHandlerDeps
	resp   *stubResponseWriter
}

func (s *UnitResourceHandlerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.opener = &stubResourceOpener{Stub: s.stub}
	s.deps = &stubUnitResourceHandlerDeps{Stub: s.stub}
	s.resp = newStubResponseWriter(s.stub)
}

func (s *UnitResourceHandlerSuite) TestIntegration(c *gc.C) {
	opened := resourcetesting.NewResource(c, s.stub, "spam", "a-service", "some data")
	s.opener.ReturnOpenResource = opened
	s.deps.ReturnNewResourceOpener = s.opener
	deps := server.NewUnitResourceHandlerDeps(s.deps)
	h := server.NewUnitResourceHandler(deps)
	req, err := api.NewHTTPDownloadRequest("spam")
	c.Assert(err, jc.ErrorIsNil)
	req.URL, err = url.ParseRequestURI("https://api:17018/units/eggs/1/resources/spam?:resource=spam")
	c.Assert(err, jc.ErrorIsNil)
	resp := &fakeResponseWriter{
		stubResponseWriter: s.resp,
	}

	c.Logf("%#v", opened.ReadCloser)
	h.ServeHTTP(resp, req)

	resp.checkWritten(c, "some data", http.Header{
		"Content-Type":   []string{api.ContentTypeRaw},
		"Content-Length": []string{"9"}, // len("some data")
		"Content-Sha384": []string{opened.Fingerprint.String()},
	})
}

func (s *UnitResourceHandlerSuite) TestNewUnitResourceHandler(c *gc.C) {
	h := server.NewUnitResourceHandler(s.deps)

	s.stub.CheckNoCalls(c)
	c.Check(h, gc.NotNil)
}

func (s *UnitResourceHandlerSuite) TestServeHTTPDownloadOkay(c *gc.C) {
	s.deps.ReturnNewResourceOpener = s.opener
	opened := resourcetesting.NewResource(c, s.stub, "spam", "a-service", "some data")
	s.deps.ReturnHandleDownload = opened
	h := &server.UnitResourceHandler{
		UnitResourceHandlerDeps: s.deps,
	}
	req, err := http.NewRequest("GET", "...", nil)
	c.Assert(err, jc.ErrorIsNil)

	h.ServeHTTP(s.resp, req)

	s.stub.CheckCallNames(c,
		"NewResourceOpener",
		"HandleDownload",
		"UpdateDownloadResponse",
		"WriteHeader",
		"Copy",
		"Close",
	)
	s.stub.CheckCall(c, 0, "NewResourceOpener", req)
	s.stub.CheckCall(c, 1, "HandleDownload", s.opener, req)
	s.stub.CheckCall(c, 2, "UpdateDownloadResponse", s.resp, opened.Resource)
	s.stub.CheckCall(c, 3, "WriteHeader", http.StatusOK)
	s.stub.CheckCall(c, 4, "Copy", s.resp, opened)
}

func (s *UnitResourceHandlerSuite) TestServeHTTPDownloadHandlerFailed(c *gc.C) {
	h := &server.UnitResourceHandler{
		UnitResourceHandlerDeps: s.deps,
	}
	failure := errors.New("<failure>")
	s.stub.SetErrors(nil, failure)
	req, err := http.NewRequest("GET", "...", nil)
	c.Assert(err, jc.ErrorIsNil)

	h.ServeHTTP(s.resp, req)

	s.stub.CheckCallNames(c,
		"NewResourceOpener",
		"HandleDownload",
		"SendHTTPError",
	)
	s.stub.CheckCall(c, 2, "SendHTTPError", s.resp, failure)
}

func (s *UnitResourceHandlerSuite) TestServeHTTPDownloadCopyFailed(c *gc.C) {
	s.deps.ReturnHandleDownload = resourcetesting.NewResource(c, s.stub, "spam", "a-service", "some data")
	h := &server.UnitResourceHandler{
		UnitResourceHandlerDeps: s.deps,
	}
	failure := errors.New("<failure>")
	s.stub.SetErrors(nil, nil, failure)
	req, err := http.NewRequest("GET", "...", nil)
	c.Assert(err, jc.ErrorIsNil)

	h.ServeHTTP(s.resp, req)

	s.stub.CheckCallNames(c,
		"NewResourceOpener",
		"HandleDownload",
		"UpdateDownloadResponse",
		"WriteHeader",
		"Copy",
		"Close",
	)
}

func (s *UnitResourceHandlerSuite) TestServeHTTPConnectFailed(c *gc.C) {
	h := &server.UnitResourceHandler{
		UnitResourceHandlerDeps: s.deps,
	}
	failure := errors.New("<failure>")
	s.stub.SetErrors(failure)
	req, err := http.NewRequest("GET", "...", nil)
	c.Assert(err, jc.ErrorIsNil)

	h.ServeHTTP(s.resp, req)

	s.stub.CheckCallNames(c,
		"NewResourceOpener",
		"SendHTTPError",
	)
	s.stub.CheckCall(c, 1, "SendHTTPError", s.resp, failure)
}

func (s *UnitResourceHandlerSuite) TestServeHTTPUnsupportedMethod(c *gc.C) {
	h := &server.UnitResourceHandler{
		UnitResourceHandlerDeps: s.deps,
	}
	req, err := http.NewRequest("HEAD", "...", nil)
	c.Assert(err, jc.ErrorIsNil)

	h.ServeHTTP(s.resp, req)

	s.stub.CheckCallNames(c,
		"NewResourceOpener",
		"SendHTTPError",
	)
}

type stubUnitResourceHandlerDeps struct {
	*testing.Stub

	ReturnNewResourceOpener resource.Opener
	ReturnHandleDownload    resource.Opened
}

func (s *stubUnitResourceHandlerDeps) NewResourceOpener(req *http.Request) (resource.Opener, error) {
	s.AddCall("NewResourceOpener", req)
	if err := s.NextErr(); err != nil {
		return nil, err
	}

	return s.ReturnNewResourceOpener, nil
}

func (s *stubUnitResourceHandlerDeps) SendHTTPError(resp http.ResponseWriter, err error) {
	s.AddCall("SendHTTPError", resp, err)
	s.NextErr() // Pop one off.
}

func (s *stubUnitResourceHandlerDeps) UpdateDownloadResponse(resp http.ResponseWriter, info resource.Resource) {
	s.AddCall("UpdateDownloadResponse", resp, info)
	s.NextErr() // Pop one off.
}

func (s *stubUnitResourceHandlerDeps) HandleDownload(opener resource.Opener, req *http.Request) (resource.Opened, error) {
	s.AddCall("HandleDownload", opener, req)
	if err := s.NextErr(); err != nil {
		return resource.Opened{}, err
	}

	return s.ReturnHandleDownload, nil
}

type stubResourceOpener struct {
	*testing.Stub

	ReturnOpenResource resource.Opened
}

func (s *stubResourceOpener) OpenResource(name string) (resource.Opened, error) {
	s.AddCall("OpenResource", name)
	if err := s.NextErr(); err != nil {
		return resource.Opened{}, err
	}

	return s.ReturnOpenResource, nil
}

func (s *stubUnitResourceHandlerDeps) Copy(w io.Writer, r io.Reader) error {
	s.AddCall("Copy", w, r)
	if err := s.NextErr(); err != nil {
		return err
	}

	return nil
}

type stubResponseWriter struct {
	*testing.Stub
	io.Writer
	buf *bytes.Buffer

	ReturnHeader http.Header
}

func newStubResponseWriter(stub *testing.Stub) *stubResponseWriter {
	writer, buf := filetesting.NewStubWriter(stub)
	return &stubResponseWriter{
		Stub:   stub,
		Writer: writer,
		buf:    buf,

		ReturnHeader: make(http.Header),
	}
}

func (s *stubResponseWriter) Header() http.Header {
	s.AddCall("Header")
	s.NextErr() // Pop one off.

	return s.ReturnHeader
}

func (s *stubResponseWriter) WriteHeader(code int) {
	s.AddCall("WriteHeader", code)
	s.NextErr() // Pop one off.
}

type fakeResponseWriter struct {
	*stubResponseWriter

	writeCalled   bool
	writtenHeader http.Header
}

func (f *fakeResponseWriter) checkWritten(c *gc.C, body string, header http.Header) {
	if !c.Check(f.writeCalled, jc.IsTrue) {
		return
	}
	c.Check(f.buf.String(), gc.Equals, body)
	c.Check(f.writtenHeader, jc.DeepEquals, header)
	c.Check(f.writtenHeader.Get("Content-Length"), gc.Equals, fmt.Sprint(len(body)))
}

func (f *fakeResponseWriter) WriteHeader(code int) {
	f.stubResponseWriter.WriteHeader(code)

	// See http.Header.clone() in the stdlib (net/http/header.go).
	header := make(http.Header)
	for k, vv := range f.ReturnHeader {
		vv2 := make([]string, len(vv))
		copy(vv2, vv)
		header[k] = vv2
	}
	f.writtenHeader = header
}

func (f *fakeResponseWriter) Write(data []byte) (int, error) {
	f.writeCalled = true
	if f.writtenHeader == nil {
		f.WriteHeader(http.StatusOK)
	}
	return f.stubResponseWriter.Write(data)
}
