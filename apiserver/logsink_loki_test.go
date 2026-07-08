// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/services"
	coretesting "github.com/juju/juju/internal/testing"
)

type logSinkLokiSuite struct {
	coretesting.BaseSuite
}

func TestLogSinkLokiSuite(t *testing.T) {
	tc.Run(t, &logSinkLokiSuite{})
}

func (s *logSinkLokiSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (s *logSinkLokiSuite) TestLogSink503WrenchDisabled(c *tc.C) {
	s.PatchValue(&logSink503WrenchActive, func() bool {
		return false
	})

	called := false
	handler := maybeWrapLogSink503Wrench(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/model/uuid/logsink", nil))

	c.Check(called, tc.IsTrue)
	c.Check(recorder.Code, tc.Equals, http.StatusNoContent)
}

func (s *logSinkLokiSuite) TestLogSink503WrenchEnabled(c *tc.C) {
	s.PatchValue(&logSink503WrenchActive, func() bool {
		return true
	})

	called := false
	handler := maybeWrapLogSink503Wrench(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/model/uuid/logsink", nil))

	c.Check(called, tc.IsFalse)
	c.Check(recorder.Code, tc.Equals, http.StatusServiceUnavailable)
	c.Check(recorder.Body.String(), tc.Equals, "logsink unavailable\n")
}

func (s *logSinkLokiSuite) TestLogSink503LokiDisabled(c *tc.C) {
	s.PatchValue(&lokiForwardingEnabled, func(ctx context.Context, _ services.ControllerDomainServices) bool {
		return false
	})

	called := false
	handler := maybeWrapLogSink503IfLokiEnabled(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}), nil)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/model/uuid/logsink", nil))

	c.Check(called, tc.IsTrue)
	c.Check(recorder.Code, tc.Equals, http.StatusNoContent)
}

func (s *logSinkLokiSuite) TestLogSink503LokiEnabled(c *tc.C) {
	s.PatchValue(&lokiForwardingEnabled, func(ctx context.Context, _ services.ControllerDomainServices) bool {
		return true
	})

	called := false
	handler := maybeWrapLogSink503IfLokiEnabled(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}), nil)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/model/uuid/logsink", nil))

	c.Check(called, tc.IsFalse)
	c.Check(recorder.Code, tc.Equals, http.StatusServiceUnavailable)
	c.Check(recorder.Body.String(), tc.Equals, "logsink unavailable\n")
}

func (s *logSinkLokiSuite) TestLogSink503LokiEnabledPreservesMethod(c *tc.C) {
	s.PatchValue(&lokiForwardingEnabled, func(ctx context.Context, _ services.ControllerDomainServices) bool {
		return true
	})

	handler := maybeWrapLogSink503IfLokiEnabled(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), nil)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/model/uuid/logsink", nil))

	c.Check(recorder.Code, tc.Equals, http.StatusServiceUnavailable)
}
