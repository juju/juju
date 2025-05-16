// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jwtparser

import (
	"io"
	"net/http"
	"strings"
	stdtesting "testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/internal/testhelpers"
)

type workerSuite struct {
	testhelpers.IsolationSuite
	client           *MockHTTPClient
	controllerConfig *MockControllerConfigService
}

func TestWorkerSuite(t *stdtesting.T) { tc.Run(t, &workerSuite{}) }
func (s *workerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.client = NewMockHTTPClient(ctrl)
	s.controllerConfig = NewMockControllerConfigService(ctrl)
	return ctrl
}

func (s *workerSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	defer s.setupMocks(c).Finish()
}

// TestJWTParserWorkerWithNoConfig tests that NewWorker function
// creates a non-nil JWTParser when the login-refresh-url config
// option is *not* set.
func (s *workerSuite) TestJWTParserWorkerWithNoConfig(c *tc.C) {
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
	s.client.EXPECT().Get(gomock.Any()).Return(&http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"keys":[]}`)),
	}, nil)
	s.controllerConfig.EXPECT().ControllerConfig(gomock.Any()).Return(controller.Config{
		"login-token-refresh-url": "https://example.com",
	}, nil)

	w, err := NewWorker(s.controllerConfig, s.client)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(workertest.CheckKill(c, w), tc.ErrorIsNil)

	parserWorker, ok := w.(*jwtParserWorker)
	c.Assert(ok, tc.IsTrue)
	c.Assert(parserWorker.jwtParser, tc.Not(tc.IsNil))
}
