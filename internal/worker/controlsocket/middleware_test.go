// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build linux

package controlsocket

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	stdtesting "testing"

	"github.com/juju/tc"
)

func TestMiddlewareSuite(t *stdtesting.T) {
	tc.Run(t, &middlewareSuite{})
}

type middlewareSuite struct{}

func (*middlewareSuite) TestCloseRequestBodyMiddlewareClosesBody(c *tc.C) {
	body := &closeTrackingBody{Reader: bytes.NewReader([]byte("payload"))}
	req := httptest.NewRequest(http.MethodPost, "/", body)
	resp := httptest.NewRecorder()

	nextCalled := false
	h := closeRequestBodyMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusNoContent)
	}))

	h.ServeHTTP(resp, req)

	c.Assert(nextCalled, tc.IsTrue)
	c.Assert(body.closed, tc.IsTrue)
}

func (*middlewareSuite) TestContentTypeMiddlewareRejectsNonJSON(c *tc.C) {
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte("{}")))
	resp := httptest.NewRecorder()

	nextCalled := false
	called := false
	var gotStatus int
	var gotErr error

	h := contentTypeMiddleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
		}),
		errorResponseWriter(func(_ context.Context, _ http.ResponseWriter, status int, err error) {
			called = true
			gotStatus = status
			gotErr = err
		}),
	)

	h.ServeHTTP(resp, req)

	c.Assert(nextCalled, tc.IsFalse)
	c.Assert(called, tc.IsTrue)
	c.Assert(gotStatus, tc.Equals, http.StatusUnsupportedMediaType)
	c.Assert(gotErr, tc.ErrorMatches, "request Content-Type must be application/json")
}

func (*middlewareSuite) TestContentTypeMiddlewareAcceptsJSON(c *tc.C) {
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp := httptest.NewRecorder()

	nextCalled := false
	writerCalled := false

	h := contentTypeMiddleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
			w.WriteHeader(http.StatusNoContent)
		}),
		errorResponseWriter(func(_ context.Context, _ http.ResponseWriter, _ int, _ error) {
			writerCalled = true
		}),
	)

	h.ServeHTTP(resp, req)

	c.Assert(nextCalled, tc.IsTrue)
	c.Assert(writerCalled, tc.IsFalse)
	c.Assert(resp.Code, tc.Equals, http.StatusNoContent)
}

func (*middlewareSuite) TestContentLengthMiddlewareRejectsOversizedContentLength(c *tc.C) {
	body := bytes.Repeat([]byte("x"), maxPayloadBytes+1)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	resp := httptest.NewRecorder()

	nextCalled := false
	called := false
	var gotStatus int
	var gotErr error

	h := contentLengthMiddleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
		}),
		errorResponseWriter(func(_ context.Context, _ http.ResponseWriter, status int, err error) {
			called = true
			gotStatus = status
			gotErr = err
		}),
	)

	h.ServeHTTP(resp, req)

	c.Assert(nextCalled, tc.IsFalse)
	c.Assert(called, tc.IsTrue)
	c.Assert(gotStatus, tc.Equals, http.StatusRequestEntityTooLarge)
	c.Assert(gotErr, tc.ErrorMatches, "request body must not exceed .* bytes")
}

func (*middlewareSuite) TestContentLengthMiddlewareMaxBytesReaderEnforced(c *tc.C) {
	body := bytes.Repeat([]byte("x"), maxPayloadBytes+1)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	// Unknown length bypasses the early ContentLength check.
	req.ContentLength = -1
	resp := httptest.NewRecorder()

	nextCalled := false
	writerCalled := false

	h := contentLengthMiddleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
			_, err := io.ReadAll(r.Body)
			c.Assert(err, tc.ErrorMatches, "http: request body too large")
			w.WriteHeader(http.StatusNoContent)
		}),
		errorResponseWriter(func(_ context.Context, _ http.ResponseWriter, _ int, _ error) {
			writerCalled = true
		}),
	)

	h.ServeHTTP(resp, req)

	c.Assert(nextCalled, tc.IsTrue)
	c.Assert(writerCalled, tc.IsFalse)
	c.Assert(resp.Code, tc.Equals, http.StatusNoContent)
}

type closeTrackingBody struct {
	io.Reader
	closed bool
}

func (b *closeTrackingBody) Close() error {
	b.closed = true
	return nil
}
