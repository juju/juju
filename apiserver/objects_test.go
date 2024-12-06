// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/apiserverhttp"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/internal/testing"
)

type objectsCharmHandlerSuite struct {
	applicationsServiceGetter *MockApplicationServiceGetter
	applicationsService       *MockApplicationService

	mux *apiserverhttp.Mux
	srv *httptest.Server
}

var _ = gc.Suite(&objectsCharmHandlerSuite{})

func (s *objectsCharmHandlerSuite) SetUpTest(c *gc.C) {
	s.mux = apiserverhttp.NewMux()
	s.srv = httptest.NewServer(s.mux)
}

func (s *objectsCharmHandlerSuite) TearDownTest(c *gc.C) {
	s.srv.Close()
}

func (s *objectsCharmHandlerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.applicationsServiceGetter = NewMockApplicationServiceGetter(ctrl)
	s.applicationsService = NewMockApplicationService(ctrl)
	s.applicationsServiceGetter.EXPECT().Application(gomock.Any()).Return(s.applicationsService, nil)

	return ctrl
}

func (s *objectsCharmHandlerSuite) TestServeGet(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.applicationsService.EXPECT().GetCharmArchiveBySHA256Prefix(gomock.Any(), "0abcdef").Return(io.NopCloser(strings.NewReader("charm-content")), nil)

	handlers := &objectsCharmHTTPHandler{
		applicationServiceGetter: s.applicationsServiceGetter,
	}

	s.mux.AddHandler("GET", charmsObjectsRoutePrefix, handlers)
	defer s.mux.RemoveHandler("GET", charmsObjectsRoutePrefix)

	modelUUID := testing.ModelTag.Id()
	hashPrefix := "0abcdef"

	resp, err := http.Get(fmt.Sprintf("%s/model-%s/charms/testcharm-%s", s.srv.URL, modelUUID, hashPrefix))
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(resp.StatusCode, gc.Equals, http.StatusOK)
	body, err := io.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(body), gc.Equals, "charm-content")
}

func (s *objectsCharmHandlerSuite) TestServeGetNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.applicationsService.EXPECT().GetCharmArchiveBySHA256Prefix(gomock.Any(), "0abcdef").Return(nil, applicationerrors.CharmNotFound)

	handlers := &objectsCharmHTTPHandler{
		applicationServiceGetter: s.applicationsServiceGetter,
	}

	s.mux.AddHandler("GET", charmsObjectsRoutePrefix, handlers)
	defer s.mux.RemoveHandler("GET", charmsObjectsRoutePrefix)

	modelUUID := testing.ModelTag.Id()
	hashPrefix := "0abcdef"

	resp, err := http.Get(fmt.Sprintf("%s/model-%s/charms/testcharm-%s", s.srv.URL, modelUUID, hashPrefix))
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(resp.StatusCode, gc.Equals, http.StatusNotFound)
}
