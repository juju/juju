// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server_test

import (
	"io"
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource/api/private/server"
	"github.com/juju/juju/resource/resourcetesting"
)

var _ = gc.Suite(&DownloadSuite{})

type DownloadSuite struct {
	testing.IsolationSuite

	stub     *testing.Stub
	store    *stubUnitDataStore
	csClient *stubCharmstoreClient
	deps     *stubDownloadDeps
}

func (s *DownloadSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.store = &stubUnitDataStore{Stub: s.stub}
	s.csClient = &stubCharmstoreClient{Stub: s.stub}
	s.deps = &stubDownloadDeps{
		Stub:                      s.stub,
		stubUnitDataStore:         s.store,
		ReturnNewCharmstoreClient: s.csClient,
	}
}

func (s *DownloadSuite) TestHandleDownloadOkay(c *gc.C) {
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

func (s *DownloadSuite) TestHandleDownloadCharmstore(c *gc.C) {
	opened := resourcetesting.NewResource(c, s.stub, "spam", "a-service", "some data")
	opened.Resource.Revision = 5
	opened.Resource.Origin = charmresource.OriginStore
	s.deps.ReturnExtractDownloadRequest = "spam"
	s.store.ReturnGetResource = opened.Resource
	s.csClient.ReturnGetResource = opened.ReadCloser
	req, err := http.NewRequest("GET", "...", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.stub.SetErrors(nil, errors.NotFoundf(""))

	res, reader, err := server.HandleDownload(req, s.deps)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c,
		"ExtractDownloadRequest",
		"OpenResource",
		"GetResource",
		"NewCharmstoreClient",
		"GetResource",
		"Close",
	)
	c.Check(res, jc.DeepEquals, opened.Resource)
	c.Check(reader, gc.Equals, opened.ReadCloser)
}

type stubDownloadDeps struct {
	*testing.Stub
	*stubUnitDataStore

	ReturnExtractDownloadRequest string
	ReturnNewCharmstoreClient    server.CharmstoreClient
}

func (s *stubDownloadDeps) ExtractDownloadRequest(req *http.Request) string {
	s.AddCall("ExtractDownloadRequest", req)
	s.NextErr() // Pop one off.

	return s.ReturnExtractDownloadRequest
}

func (s *stubDownloadDeps) NewCharmstoreClient() (server.CharmstoreClient, error) {
	s.AddCall("NewCharmstoreClient")
	if err := s.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.ReturnNewCharmstoreClient, nil
}

type stubCharmstoreClient struct {
	*testing.Stub

	ReturnGetResource io.ReadCloser
}

func (s *stubCharmstoreClient) GetResource(name string) (io.ReadCloser, error) {
	s.AddCall("GetResource", name)
	if err := s.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.ReturnGetResource, nil
}

func (s *stubCharmstoreClient) Close() error {
	s.AddCall("Close")
	if err := s.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}
