// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
)

type logSinkWrenchSuite struct {
	coretesting.BaseSuite
}

func TestLogSinkWrenchSuite(t *testing.T) {
	tc.Run(t, &logSinkWrenchSuite{})
}

func (s *logSinkWrenchSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (s *logSinkWrenchSuite) TestLogSink503WrenchDisabled(c *tc.C) {
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

func (s *logSinkWrenchSuite) TestLogSink503WrenchEnabled(c *tc.C) {
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
