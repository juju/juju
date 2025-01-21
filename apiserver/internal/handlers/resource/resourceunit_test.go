// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"

	"github.com/juju/names/v6"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	resource "github.com/juju/juju/apiserver/internal/handlers/resource"
	coreresource "github.com/juju/juju/core/resource"
	charmresource "github.com/juju/juju/internal/charm/resource"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type UnitResourcesHandlerSuite struct {
	testing.IsolationSuite

	opener       *MockOpener
	openerGetter *MockResourceOpenerGetter

	urlStr   string
	recorder *httptest.ResponseRecorder
}

var _ = gc.Suite(&UnitResourcesHandlerSuite{})

func (s *UnitResourcesHandlerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.opener = NewMockOpener(ctrl)
	s.openerGetter = NewMockResourceOpenerGetter(ctrl)

	return ctrl
}

func (s *UnitResourcesHandlerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	args := url.Values{}
	args.Add(":unit", "foo/0")
	args.Add(":resource", "blob")
	s.urlStr = "https://api:17017/?" + args.Encode()

	s.recorder = httptest.NewRecorder()
}

func (s *UnitResourcesHandlerSuite) newUnitResourceHander(c *gc.C) *UnitResourcesHandler {
	s.openerGetter.EXPECT().Opener(gomock.Any(), names.UnitTagKind, names.ApplicationTagKind).Return(s.opener, nil)
	return NewUnitResourcesHandler(
		loggertesting.WrapCheckLog(c),
		s.openerGetter,
	)
}

func (s *UnitResourcesHandlerSuite) TestWrongMethod(c *gc.C) {
	defer s.setupMocks(c).Finish()
	handler := NewUnitResourcesHandler(
		loggertesting.WrapCheckLog(c),
		nil,
	)

	req, err := http.NewRequest("POST", s.urlStr, nil)
	c.Assert(err, jc.ErrorIsNil)

	handler.ServeHTTP(s.recorder, req)

	c.Assert(s.recorder.Code, gc.Equals, http.StatusMethodNotAllowed)
}

func (s *UnitResourcesHandlerSuite) TestOpenerCreationError(c *gc.C) {
	defer s.setupMocks(c).Finish()
	failure, expectedBody := apiFailure("boom", "")
	s.openerGetter.EXPECT().Opener(gomock.Any(), names.UnitTagKind, names.ApplicationTagKind).Return(nil, failure)
	handler := resource.NewUnitResourcesHandler(
		loggertesting.WrapCheckLog(c),
		s.openerGetter,
	)

	req, err := http.NewRequest("GET", s.urlStr, nil)
	c.Assert(err, jc.ErrorIsNil)

	handler.ServeHTTP(s.recorder, req)

	s.checkResp(c,
		http.StatusInternalServerError,
		"application/json",
		expectedBody,
	)
}

func (s *UnitResourcesHandlerSuite) TestOpenResourceError(c *gc.C) {
	defer s.setupMocks(c).Finish()
	failure, expectedBody := apiFailure("boom", "")
	handler := s.newUnitResourceHander(c)
	s.opener.EXPECT().OpenResource(gomock.Any(), "blob").Return(coreresource.Opened{}, failure)

	req, err := http.NewRequest("GET", s.urlStr, nil)
	c.Assert(err, jc.ErrorIsNil)

	handler.ServeHTTP(s.recorder, req)

	s.checkResp(c, http.StatusInternalServerError, "application/json", expectedBody)
}

func (s *UnitResourcesHandlerSuite) TestSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()
	const body = "some data"
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(body))
	c.Assert(err, jc.ErrorIsNil)
	size := int64(len(body))
	handler := s.newUnitResourceHander(c)

	opened := coreresource.Opened{
		Resource: coreresource.Resource{
			Resource: charmresource.Resource{
				Fingerprint: fp,
				Size:        size,
			},
		},
		ReadCloser: io.NopCloser(strings.NewReader(body)),
	}
	s.opener.EXPECT().OpenResource(gomock.Any(), "blob").Return(opened, nil)
	s.opener.EXPECT().SetResourceUsed(gomock.Any(), "blob").Return(nil)

	req, err := http.NewRequest("GET", s.urlStr, nil)
	c.Assert(err, jc.ErrorIsNil)

	handler.ServeHTTP(s.recorder, req)

	s.checkResp(c, http.StatusOK, "application/octet-stream", body)
}

func (s *UnitResourcesHandlerSuite) checkResp(c *gc.C, status int, ctype, body string) {
	c.Assert(s.recorder.Code, gc.Equals, status)
	hdr := s.recorder.Header()
	c.Check(hdr.Get("Content-Type"), gc.Equals, ctype)
	c.Check(hdr.Get("Content-Length"), gc.Equals, strconv.Itoa(len(body)))

	actualBody, err := io.ReadAll(s.recorder.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(actualBody), gc.Equals, body)
}
