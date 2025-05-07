// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objects

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/apiserverhttp"
	objectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
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

var _ = tc.Suite(&objectsHandlerSuite{})

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
	c.Assert(err, jc.ErrorIsNil)
	c.Check(resp.StatusCode, tc.Equals, http.StatusNotImplemented)
}

func (s *objectsHandlerSuite) TestServeGet(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectObjectStore()

	reader := io.NopCloser(strings.NewReader("charm-content"))
	s.objectStore.EXPECT().GetBySHA256(gomock.Any(), "fab5b76e7c234d9c929014d46ef0a5db9c8b6e9fd63bdc3ba9c2b903471bc77e").Return(reader, 13, nil)

	handlers := &ObjectsHTTPHandler{
		objectStoreGetter: s.objectStoreGetter,
	}

	s.mux.AddHandler("GET", objectsRoutePrefix, handlers)
	defer s.mux.RemoveHandler("GET", objectsRoutePrefix)

	modelUUID := testing.ModelTag.Id()
	hash := "fab5b76e7c234d9c929014d46ef0a5db9c8b6e9fd63bdc3ba9c2b903471bc77e"

	url := fmt.Sprintf("%s/model-%s/objects/%s", s.srv.URL, modelUUID, hash)
	resp, err := http.Get(url)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(resp.StatusCode, tc.Equals, http.StatusOK)
	body, err := io.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(body), tc.Equals, "charm-content")
}

func (s *objectsHandlerSuite) TestServeGetInvalidSize(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectObjectStore()

	reader := io.NopCloser(strings.NewReader("charm-content"))
	s.objectStore.EXPECT().GetBySHA256(gomock.Any(), "fab5b76e7c234d9c929014d46ef0a5db9c8b6e9fd63bdc3ba9c2b903471bc77e").Return(reader, 2, nil)

	handlers := &ObjectsHTTPHandler{
		objectStoreGetter: s.objectStoreGetter,
	}

	s.mux.AddHandler("GET", objectsRoutePrefix, handlers)
	defer s.mux.RemoveHandler("GET", objectsRoutePrefix)

	modelUUID := testing.ModelTag.Id()
	hash := "fab5b76e7c234d9c929014d46ef0a5db9c8b6e9fd63bdc3ba9c2b903471bc77e"

	url := fmt.Sprintf("%s/model-%s/objects/%s", s.srv.URL, modelUUID, hash)
	resp, err := http.Get(url)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(resp.StatusCode, tc.Equals, http.StatusOK)
	_, err = io.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIs, io.ErrUnexpectedEOF)
}

func (s *objectsHandlerSuite) TestServeGetNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectObjectStore()

	s.objectStore.EXPECT().GetBySHA256(gomock.Any(), "fab5b76e7c234d9c929014d46ef0a5db9c8b6e9fd63bdc3ba9c2b903471bc77e").Return(nil, -1, objectstoreerrors.ErrNotFound)

	handlers := &ObjectsHTTPHandler{
		objectStoreGetter: s.objectStoreGetter,
	}

	s.mux.AddHandler("GET", objectsRoutePrefix, handlers)
	defer s.mux.RemoveHandler("GET", objectsRoutePrefix)

	modelUUID := testing.ModelTag.Id()
	hash := "fab5b76e7c234d9c929014d46ef0a5db9c8b6e9fd63bdc3ba9c2b903471bc77e"

	url := fmt.Sprintf("%s/model-%s/objects/%s", s.srv.URL, modelUUID, hash)
	resp, err := http.Get(url)
	c.Assert(err, jc.ErrorIsNil)
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
