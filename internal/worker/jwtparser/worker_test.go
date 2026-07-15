// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jwtparser

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/internal/testhelpers"
)

type workerSuite struct {
	testhelpers.IsolationSuite
	client           *MockHTTPClient
	controllerConfig *MockControllerConfigService
}

func TestWorkerSuite(t *testing.T) {
	tc.Run(t, &workerSuite{})
}

func (s *workerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.client = NewMockHTTPClient(ctrl)
	s.controllerConfig = NewMockControllerConfigService(ctrl)
	return ctrl
}

// TestJWTParserWorkerWithNoConfig tests that NewWorker function
// creates a non-nil JWTParser when the login-refresh-url config
// option is *not* set.
func (s *workerSuite) TestJWTParserWorkerWithNoConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.controllerConfig.EXPECT().ControllerConfig(gomock.Any()).Return(controller.Config{}, nil)

	w, err := NewWorker(s.controllerConfig, s.client)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(workertest.CheckKill(c, w), tc.ErrorIsNil)

	parserWorker, ok := w.(*jwtParserWorker)
	c.Assert(ok, tc.IsTrue)
	c.Assert(parserWorker.jwtParser, tc.Not(tc.IsNil))
}

// TestJWTParserWorkerWithLoginRefreshURL tests that NewWorker function
// creates a non-nil JWTParser when the login-refresh-url config option is set.
func (s *workerSuite) TestJWTParserWorkerWithLoginRefreshURL(c *tc.C) {
	defer s.setupMocks(c).Finish()
	refreshURL := "https://example.com/keys"
	parsedURL, err := url.Parse(refreshURL)
	c.Assert(err, tc.ErrorIsNil)

	s.client.EXPECT().Do(gomock.Any()).DoAndReturn(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"keys":[]}`)),
			Request:    &http.Request{URL: parsedURL},
		}, nil
	}).AnyTimes()
	s.controllerConfig.EXPECT().ControllerConfig(gomock.Any()).Return(controller.Config{
		"login-token-refresh-url": refreshURL,
	}, nil)

	w, err := NewWorker(s.controllerConfig, s.client)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(workertest.CheckKill(c, w), tc.ErrorIsNil)

	parserWorker, ok := w.(*jwtParserWorker)
	c.Assert(ok, tc.IsTrue)
	c.Assert(parserWorker.jwtParser, tc.Not(tc.IsNil))
}
