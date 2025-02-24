// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jwtparser

import (
	"io"
	"net/http"
	"strings"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/worker/jwtparser/mocks"
)

type workerSuite struct {
	testing.IsolationSuite
	client           *mocks.MockHTTPClient
	controllerConfig *mocks.MockControllerConfig
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	ctrl := gomock.NewController(c)
	s.client = mocks.NewMockHTTPClient(ctrl)
	s.controllerConfig = mocks.NewMockControllerConfig(ctrl)
}

// TestJWTParserWorkerWithNoConfig tests that NewWorker function
// creates a JWTParserWorker when the login-refresh-url config
// option is not set.
func (s *workerSuite) TestJWTParserWorkerWithNoConfig(c *gc.C) {
	s.controllerConfig.EXPECT().ControllerConfig().Return(controller.Config{}, nil)

	w, err := NewWorker(s.controllerConfig, s.client)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(workertest.CheckKill(c, w), jc.ErrorIsNil)

	parserWorker, ok := w.(*jwtParserWorker)
	c.Assert(ok, jc.IsTrue)
	c.Assert(parserWorker.Get(), gc.IsNil)
}

// TestJWTParserWorkerWithLoginRefreshURL tests that NewWorker function
// creates a JWTParserWorker when the login-refresh-url config option is set.
func (s *workerSuite) TestJWTParserWorkerWithLoginRefreshURL(c *gc.C) {
	s.client.EXPECT().Get(gomock.Any()).Return(&http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"keys":[]}`)),
	}, nil)
	s.controllerConfig.EXPECT().ControllerConfig().Return(controller.Config{
		"login-token-refresh-url": "https://example.com",
	}, nil)

	w, err := NewWorker(s.controllerConfig, s.client)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(workertest.CheckKill(c, w), jc.ErrorIsNil)

	parserWorker, ok := w.(*jwtParserWorker)
	c.Assert(ok, jc.IsTrue)
	c.Assert(parserWorker.Get(), gc.Not(gc.IsNil))
}
