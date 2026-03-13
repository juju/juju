// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objects

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/core/objectstore"
	domainobjectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	objectstoreerrors "github.com/juju/juju/internal/objectstore/errors"
	"github.com/juju/juju/internal/testing"
)

const (
	objectsRoutePrefix = "/model-:modeluuid/objects/:object"
)

type objectsHandlerSuite struct {
	objectStoreGetter *MockObjectStoreServiceGetter
	objectStore       *MockObjectStoreService

	mux *apiserverhttp.Mux
	srv *httptest.Server
}

func TestObjectsHandlerSuite(t *stdtesting.T) {
	tc.Run(t, &objectsHandlerSuite{})
}

func (s *objectsHandlerSuite) SetUpTest(c *tc.C) {
	s.mux = apiserverhttp.NewMux()
	s.srv = httptest.NewServer(s.mux)
}

func (s *objectsHandlerSuite) TearDownTest(c *tc.C) {
	s.srv.Close()
}

func (s *objectsHandlerSuite) TestServeMethodNotSupported(c *tc.C) {
	defer s.setupMocks(c).Finish()

	handlers := &ObjectsHTTPHandler{
		objectStoreGetter: s.objectStoreGetter,
	}

	// This is a bit pathological, but we want to make sure that the handler
	// logic only actions on POST requests.
	s.mux.AddHandler("POST", objectsRoutePrefix, handlers)
	defer s.mux.RemoveHandler("POST", objectsRoutePrefix)

	modelUUID := testing.ModelTag.Id()
	hash := "fab5b76e7c234d9c929014d46ef0a5db9c8b6e9fd63bdc3ba9c2b903471bc77e"

	url := fmt.Sprintf("%s/model-%s/objects/%s", s.srv.URL, modelUUID, hash)
	resp, err := http.Post(url, "application/octet-stream", strings.NewReader("charm-content"))
	c.Assert(err, tc.ErrorIsNil)
	defer resp.Body.Close()
	c.Check(resp.StatusCode, tc.Equals, http.StatusNotImplemented)
}

func (s *objectsHandlerSuite) TestServeGet(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectObjectStore()

	hash := "fab5b76e7c234d9c929014d46ef0a5db9c8b6e9fd63bdc3ba9c2b903471bc77e"

	reader := io.NopCloser(strings.NewReader("charm-content"))
	s.objectStore.EXPECT().GetBySHA256(gomock.Any(), hash).Return(reader, objectstore.Digest{
		SHA256: hash,
		Size:   13,
	}, nil)

	handlers := &ObjectsHTTPHandler{
		objectStoreGetter: s.objectStoreGetter,
	}

	s.mux.AddHandler("GET", objectsRoutePrefix, handlers)
	defer s.mux.RemoveHandler("GET", objectsRoutePrefix)

	modelUUID := testing.ModelTag.Id()

	url := fmt.Sprintf("%s/model-%s/objects/%s", s.srv.URL, modelUUID, hash)
	resp, err := http.Get(url)
	c.Assert(err, tc.ErrorIsNil)
	defer resp.Body.Close()

	c.Assert(resp.StatusCode, tc.Equals, http.StatusOK)
	body, err := io.ReadAll(resp.Body)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(body), tc.Equals, "charm-content")
}

func (s *objectsHandlerSuite) TestServeGetHeaders(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectObjectStore()

	hash := "fab5b76e7c234d9c929014d46ef0a5db9c8b6e9fd63bdc3ba9c2b903471bc77e"

	reader := io.NopCloser(strings.NewReader("charm-content"))
	s.objectStore.EXPECT().GetBySHA256(gomock.Any(), hash).Return(reader, objectstore.Digest{
		SHA256: hash,
		Size:   13,
	}, nil)

	handlers := &ObjectsHTTPHandler{
		objectStoreGetter: s.objectStoreGetter,
	}

	s.mux.AddHandler("GET", objectsRoutePrefix, handlers)
	defer s.mux.RemoveHandler("GET", objectsRoutePrefix)

	modelUUID := testing.ModelTag.Id()

	url := fmt.Sprintf("%s/model-%s/objects/%s", s.srv.URL, modelUUID, hash)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	c.Assert(err, tc.ErrorIsNil)
	req.Header.Set("x-amz-request-id", "1234")
	req.Header.Set("x-amz-id-2", "5678")

	resp, err := http.DefaultClient.Do(req)
	c.Assert(err, tc.ErrorIsNil)
	defer resp.Body.Close()

	c.Check(resp.Header.Get("x-amzn-requestid"), tc.Equals, "1234")
	c.Check(resp.Header.Get("x-amzn-id-2"), tc.Equals, "5678")
	c.Check(resp.Header.Get("x-amz-checksum-sha256"), tc.Equals, "+rW3bnwjTZySkBTUbvCl25yLbp/WO9w7qcK5A0cbx34=")
}

func (s *objectsHandlerSuite) TestServeGetInvalidHash(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectObjectStore()

	hash := "fab5b76e7c234d9c929014d46ef0a5db9c8b6e9fd63bdc3ba9c2b903471bc77e"

	reader := io.NopCloser(strings.NewReader("charm-content"))
	s.objectStore.EXPECT().GetBySHA256(gomock.Any(), hash).Return(reader, objectstore.Digest{
		SHA256: hash,
		Size:   13,
	}, domainobjectstoreerrors.ErrInvalidHashLength)

	handlers := &ObjectsHTTPHandler{
		objectStoreGetter: s.objectStoreGetter,
	}

	s.mux.AddHandler("GET", objectsRoutePrefix, handlers)
	defer s.mux.RemoveHandler("GET", objectsRoutePrefix)

	modelUUID := testing.ModelTag.Id()

	url := fmt.Sprintf("%s/model-%s/objects/%s", s.srv.URL, modelUUID, hash)
	resp, err := http.Get(url)
	c.Assert(err, tc.ErrorIsNil)
	defer resp.Body.Close()

	c.Assert(resp.StatusCode, tc.Equals, http.StatusBadRequest)
}

func (s *objectsHandlerSuite) TestServeGetInvalidSize(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectObjectStore()

	hash := "fab5b76e7c234d9c929014d46ef0a5db9c8b6e9fd63bdc3ba9c2b903471bc77e"

	reader := io.NopCloser(strings.NewReader("charm-content"))
	s.objectStore.EXPECT().GetBySHA256(gomock.Any(), hash).Return(reader, objectstore.Digest{
		SHA256: hash,
		Size:   2,
	}, nil)

	handlers := &ObjectsHTTPHandler{
		objectStoreGetter: s.objectStoreGetter,
	}

	s.mux.AddHandler("GET", objectsRoutePrefix, handlers)
	defer s.mux.RemoveHandler("GET", objectsRoutePrefix)

	modelUUID := testing.ModelTag.Id()

	url := fmt.Sprintf("%s/model-%s/objects/%s", s.srv.URL, modelUUID, hash)
	resp, err := http.Get(url)
	c.Assert(err, tc.ErrorIsNil)
	defer resp.Body.Close()

	c.Assert(resp.StatusCode, tc.Equals, http.StatusOK)
	_, err = io.ReadAll(resp.Body)
	c.Assert(err, tc.ErrorIs, io.ErrUnexpectedEOF)
}

func (s *objectsHandlerSuite) TestServeGetNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectObjectStore()

	hash := "fab5b76e7c234d9c929014d46ef0a5db9c8b6e9fd63bdc3ba9c2b903471bc77e"

	s.objectStore.EXPECT().GetBySHA256(gomock.Any(), hash).
		Return(nil, objectstore.Digest{}, objectstoreerrors.ObjectNotFound)

	handlers := &ObjectsHTTPHandler{
		objectStoreGetter: s.objectStoreGetter,
	}

	s.mux.AddHandler("GET", objectsRoutePrefix, handlers)
	defer s.mux.RemoveHandler("GET", objectsRoutePrefix)

	modelUUID := testing.ModelTag.Id()

	url := fmt.Sprintf("%s/model-%s/objects/%s", s.srv.URL, modelUUID, hash)
	resp, err := http.Get(url)
	c.Assert(err, tc.ErrorIsNil)
	defer resp.Body.Close()
	c.Check(resp.StatusCode, tc.Equals, http.StatusNotFound)
}

func (s *objectsHandlerSuite) expectObjectStore() {
	s.objectStoreGetter.EXPECT().ObjectStore(gomock.Any()).Return(s.objectStore, nil)
}

func (s *objectsHandlerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.objectStoreGetter = NewMockObjectStoreServiceGetter(ctrl)
	s.objectStore = NewMockObjectStoreService(ctrl)

	return ctrl
}
