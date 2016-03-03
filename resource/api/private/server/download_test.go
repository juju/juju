// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server_test

import (
	"net/http"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/resource/api/private/server"
	"github.com/juju/juju/resource/resourcetesting"
)

var _ = gc.Suite(&DownloadSuite{})

type DownloadSuite struct {
	testing.IsolationSuite

	stub  *testing.Stub
	store *stubUnitDataStore
	deps  *stubDownloadDeps
}

func (s *DownloadSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.store = &stubUnitDataStore{Stub: s.stub}
	s.deps = &stubDownloadDeps{
		Stub:              s.stub,
		stubUnitDataStore: s.store,
	}
}

func (s *DownloadSuite) TestHandleDownload(c *gc.C) {
	opened := resourcetesting.NewResource(c, s.stub, "spam", "a-service", "some data")
	s.deps.ReturnExtractDownloadRequest = "spam"
	s.store.ReturnOpenResource = opened
	req, err := http.NewRequest("GET", "...", nil)
	c.Assert(err, jc.ErrorIsNil)

	res, reader, err := server.HandleDownload(req, s.deps)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c,
		"ExtractDownloadRequest",
		"OpenResource",
	)
	c.Check(res, jc.DeepEquals, opened.Resource)
	c.Check(reader, gc.Equals, opened.ReadCloser)
}

type stubDownloadDeps struct {
	*testing.Stub
	*stubUnitDataStore

	ReturnExtractDownloadRequest string
}

func (s *stubDownloadDeps) ExtractDownloadRequest(req *http.Request) string {
	s.AddCall("ExtractDownloadRequest", req)
	s.NextErr() // Pop one off.

	return s.ReturnExtractDownloadRequest
}
