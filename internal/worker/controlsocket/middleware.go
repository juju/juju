// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build linux

package controlsocket

import (
	"context"
	"mime"
	"net/http"
	"strings"

	"github.com/juju/juju/internal/errors"
)

// MiddlewareErrorWriter is an interface that defines a method for writing error
// responses in middleware.
type MiddlewareErrorWriter interface {
	// WriteErrorResponse writes an error response to the given
	// http.ResponseWriter, using the provided status code and error message.
	WriteErrorResponse(ctx context.Context, w http.ResponseWriter, status int, err error)
}

type errorResponseWriter func(ctx context.Context, w http.ResponseWriter, status int, err error)

func (f errorResponseWriter) WriteErrorResponse(ctx context.Context, w http.ResponseWriter, status int, err error) {
	f(ctx, w, status, err)
}

// closeRequestBodyMiddleware is a middleware that ensures the request body is
// closed after the request is processed.
func closeRequestBodyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(resp http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		next.ServeHTTP(resp, r)
	})
}

// contentTypeMiddleware is a middleware that checks if the request has the
// expected Content-Type header and returns an error response if it does not.
func contentTypeMiddleware(next http.Handler, errorWriter MiddlewareErrorWriter) http.Handler {
	return http.HandlerFunc(func(resp http.ResponseWriter, r *http.Request) {
		if !hasContentType(r, "application/json") {
			errorWriter.WriteErrorResponse(r.Context(), resp, http.StatusUnsupportedMediaType,
				errors.New("request Content-Type must be application/json"))
			return
		}
		next.ServeHTTP(resp, r)
	})
}

// contentLengthMiddleware is a middleware that checks if the request body
// exceeds a specified maximum length and returns an error response if it does.
func contentLengthMiddleware(next http.Handler, errorWriter MiddlewareErrorWriter) http.Handler {
	return http.HandlerFunc(func(resp http.ResponseWriter, r *http.Request) {
		if r.ContentLength > maxPayloadBytes {
			errorWriter.WriteErrorResponse(r.Context(), resp, http.StatusRequestEntityTooLarge,
				errors.Errorf("request body must not exceed %d bytes", maxPayloadBytes))
			return
		}

		r.Body = http.MaxBytesReader(resp, r.Body, maxPayloadBytes)

		next.ServeHTTP(resp, r)
	})
}

func hasContentType(r *http.Request, mimeType string) bool {
	contentType := r.Header.Get("Content-type")
	if contentType == "" {
		return false
	}

	for v := range strings.SplitSeq(contentType, ",") {
		t, _, err := mime.ParseMediaType(v)
		if err != nil {
			break
		}
		if t == mimeType {
			return true
		}
	}
	return false
}
