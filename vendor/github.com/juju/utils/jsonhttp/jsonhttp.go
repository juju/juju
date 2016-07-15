// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.
// The jsonhttp package provides general functions for returning
// JSON responses to HTTP requests. It is agnostic about
// the specific form of any returned errors.
package jsonhttp

import (
	"encoding/json"
	"net/http"

	"gopkg.in/errgo.v1"
)

// ErrorToResponse represents a function that can convert a Go error
// into a form that can be returned as a JSON body from an HTTP request.
// The httpStatus value reports the desired HTTP status.
type ErrorToResponse func(err error) (httpStatus int, errorBody interface{})

// ErrorHandler is like http.Handler except it returns an error
// which may be returned as the error body of the response.
// An ErrorHandler function should not itself write to the ResponseWriter
// if it returns an error.
type ErrorHandler func(http.ResponseWriter, *http.Request) error

// HandleErrors returns a function that can be used to convert an ErrorHandler
// into an http.Handler. The given errToResp parameter is used to convert
// any non-nil error returned by handle to the response in the HTTP body.
func HandleErrors(errToResp ErrorToResponse) func(handle ErrorHandler) http.Handler {
	writeError := WriteError(errToResp)
	return func(handle ErrorHandler) http.Handler {
		f := func(w http.ResponseWriter, req *http.Request) {
			w1 := responseWriter{
				ResponseWriter: w,
			}
			if err := handle(&w1, req); err != nil {
				// We write the error only if the header hasn't
				// already been written, because if it has, then
				// we will not be able to set the appropriate error
				// response code, and there's a danger that we
				// may be corrupting output by appending
				// a JSON error message to it.
				if !w1.headerWritten {
					writeError(w, err)
				}
				// TODO log the error?
			}
		}
		return http.HandlerFunc(f)
	}
}

// responseWriter wraps http.ResponseWriter but allows us
// to find out whether any body has already been written.
type responseWriter struct {
	headerWritten bool
	http.ResponseWriter
}

func (w *responseWriter) Write(data []byte) (int, error) {
	w.headerWritten = true
	return w.ResponseWriter.Write(data)
}

func (w *responseWriter) WriteHeader(code int) {
	w.headerWritten = true
	w.ResponseWriter.WriteHeader(code)
}

// Flush implements http.Flusher.Flush.
func (w *responseWriter) Flush() {
	w.headerWritten = true
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Ensure statically that responseWriter does implement http.Flusher.
var _ http.Flusher = (*responseWriter)(nil)

// WriteError returns a function that can be used to write an error to a ResponseWriter
// and set the HTTP status code. The errToResp parameter is used to determine
// the actual error value and status to write.
func WriteError(errToResp ErrorToResponse) func(w http.ResponseWriter, err error) {
	return func(w http.ResponseWriter, err error) {
		status, resp := errToResp(err)
		WriteJSON(w, status, resp)
	}
}

// WriteJSON writes the given value to the ResponseWriter
// and sets the HTTP status to the given code.
func WriteJSON(w http.ResponseWriter, code int, val interface{}) error {
	// TODO consider marshalling directly to w using json.NewEncoder.
	// pro: this will not require a full buffer allocation.
	// con: if there's an error after the first write, it will be lost.
	data, err := json.Marshal(val)
	if err != nil {
		// TODO(rog) log an error if this fails and lose the
		// error return, because most callers will need
		// to do that anyway.
		return errgo.Mask(err)
	}
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(code)
	w.Write(data)
	return nil
}

// JSONHandler is like http.Handler except that it returns a
// body (to be converted to JSON) and an error.
// The Header parameter can be used to set
// custom header on the response.
type JSONHandler func(http.Header, *http.Request) (interface{}, error)

// HandleJSON returns a function that can be used to convert an JSONHandler
// into an http.Handler. The given errToResp parameter is used to convert
// any non-nil error returned by handle to the response in the HTTP body
// If it returns a nil value, the original error is returned as a JSON string.
func HandleJSON(errToResp ErrorToResponse) func(handle JSONHandler) http.Handler {
	handleErrors := HandleErrors(errToResp)
	return func(handle JSONHandler) http.Handler {
		f := func(w http.ResponseWriter, req *http.Request) error {
			val, err := handle(w.Header(), req)
			if err != nil {
				return errgo.Mask(err, errgo.Any)
			}
			return WriteJSON(w, http.StatusOK, val)
		}
		return handleErrors(f)
	}
}
