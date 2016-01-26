// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server_test

import (
	"bytes"
	"io"
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/testing/filetesting"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api/private/server"
	"github.com/juju/juju/resource/resourcetesting"
)

var _ = gc.Suite(&LegacyHTTPHandlerSuite{})

type LegacyHTTPHandlerSuite struct {
	testing.IsolationSuite

	stub  *testing.Stub
	store *stubUnitDataStore
	deps  *stubLegacyHTTPHandlerDeps
	resp  *stubResponseWriter
}

func (s *LegacyHTTPHandlerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.store = &stubUnitDataStore{Stub: s.stub}
	s.deps = &stubLegacyHTTPHandlerDeps{Stub: s.stub}
	s.resp = newStubResponseWriter(s.stub)
}

func (s *LegacyHTTPHandlerSuite) TestNewLegacyHTTPHandler(c *gc.C) {
	h := server.NewLegacyHTTPHandler(s.deps.Connect)

	s.stub.CheckNoCalls(c)
	c.Check(h.Connect, gc.NotNil) // We can't compare functions.
	c.Check(h.HandleDownload, gc.NotNil)
}

func (s *LegacyHTTPHandlerSuite) TestServeHTTPDownloadOkay(c *gc.C) {
	s.deps.ReturnHandleDownload = resourcetesting.NewResource(c, s.stub, "spam", "some data")
	h := &server.LegacyHTTPHandler{
		LegacyHTTPHandlerDeps: s.deps,
	}
	req, err := http.NewRequest("GET", "...", nil)
	c.Assert(err, jc.ErrorIsNil)

	h.ServeHTTP(s.resp, req)

	s.stub.CheckCallNames(c,
		"Connect",
		"HandleDownload",
		"UpdateDownloadResponse",
		"WriteHeader",
		"Copy",
		"Close",
	)
}

func (s *LegacyHTTPHandlerSuite) TestServeHTTPDownloadHandlerFailed(c *gc.C) {
	h := &server.LegacyHTTPHandler{
		LegacyHTTPHandlerDeps: s.deps,
	}
	failure := errors.New("<failure>")
	s.stub.SetErrors(nil, failure)
	req, err := http.NewRequest("GET", "...", nil)
	c.Assert(err, jc.ErrorIsNil)

	h.ServeHTTP(s.resp, req)

	s.stub.CheckCallNames(c,
		"Connect",
		"HandleDownload",
		"SendHTTPError",
	)
}

func (s *LegacyHTTPHandlerSuite) TestServeHTTPDownloadCopyFailed(c *gc.C) {
	s.deps.ReturnHandleDownload = resourcetesting.NewResource(c, s.stub, "spam", "some data")
	h := &server.LegacyHTTPHandler{
		LegacyHTTPHandlerDeps: s.deps,
	}
	failure := errors.New("<failure>")
	s.stub.SetErrors(nil, nil, failure)
	req, err := http.NewRequest("GET", "...", nil)
	c.Assert(err, jc.ErrorIsNil)

	h.ServeHTTP(s.resp, req)

	s.stub.CheckCallNames(c,
		"Connect",
		"HandleDownload",
		"UpdateDownloadResponse",
		"WriteHeader",
		"Copy",
		"Close",
	)
}

func (s *LegacyHTTPHandlerSuite) TestServeHTTPConnectFailed(c *gc.C) {
	h := &server.LegacyHTTPHandler{
		LegacyHTTPHandlerDeps: s.deps,
	}
	failure := errors.New("<failure>")
	s.stub.SetErrors(failure)
	req, err := http.NewRequest("GET", "...", nil)
	c.Assert(err, jc.ErrorIsNil)

	h.ServeHTTP(s.resp, req)

	s.stub.CheckCallNames(c,
		"Connect",
		"SendHTTPError",
	)
}

func (s *LegacyHTTPHandlerSuite) TestServeHTTPUnsupportedMethod(c *gc.C) {
	h := &server.LegacyHTTPHandler{
		LegacyHTTPHandlerDeps: s.deps,
	}
	req, err := http.NewRequest("HEAD", "...", nil)
	c.Assert(err, jc.ErrorIsNil)

	h.ServeHTTP(s.resp, req)

	s.stub.CheckCallNames(c,
		"Connect",
		"SendHTTPError",
	)
}

type stubLegacyHTTPHandlerDeps struct {
	*testing.Stub

	ReturnConnect        server.UnitDataStore
	ReturnHandleDownload resource.Opened
}

func (s *stubLegacyHTTPHandlerDeps) Connect(req *http.Request) (server.UnitDataStore, error) {
	s.AddCall("Connect", req)
	if err := s.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.ReturnConnect, nil
}

func (s *stubLegacyHTTPHandlerDeps) SendHTTPError(resp http.ResponseWriter, err error) {
	s.AddCall("SendHTTPError", resp, err)
	s.NextErr() // Pop one off.
}

func (s *stubLegacyHTTPHandlerDeps) UpdateDownloadResponse(resp http.ResponseWriter, info resource.Resource) {
	s.AddCall("UpdateDownloadResponse", resp, info)
	s.NextErr() // Pop one off.
}

func (s *stubLegacyHTTPHandlerDeps) HandleDownload(st server.UnitDataStore, req *http.Request) (resource.Resource, io.ReadCloser, error) {
	s.AddCall("HandleDownload", st, req)
	if err := s.NextErr(); err != nil {
		return resource.Resource{}, nil, errors.Trace(err)
	}

	return s.ReturnHandleDownload.Resource, s.ReturnHandleDownload, nil
}

func (s *stubLegacyHTTPHandlerDeps) Copy(w io.Writer, r io.Reader) error {
	s.AddCall("Copy", w, r)
	if err := s.NextErr(); err != nil {
		return errors.Trace(err)
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
