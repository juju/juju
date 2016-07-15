// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package router // import "gopkg.in/juju/charmstore.v5-unstable/internal/router"

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"mime"
	"net/http"
	"strings"

	"github.com/juju/httprequest"
	"github.com/juju/loggo"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
)

var logger = loggo.GetLogger("charmstore.internal.router")

// WriteError can be used to write an error response.
var WriteError = errorToResp.WriteError

// JSONHandler represents a handler that returns a JSON value.
// The provided header can be used to set response headers.
type JSONHandler func(http.Header, *http.Request) (interface{}, error)

// ErrorHandler represents a handler that can return an error.
type ErrorHandler func(http.ResponseWriter, *http.Request) error

// HandleJSON converts from a JSONHandler function to an http.Handler.
func HandleJSON(h JSONHandler) http.Handler {
	// We can't use errorToResp.HandleJSON directly because
	// we still use old-style handlers in charmstore, so we
	// insert shim functions to do the conversion.
	handleJSON := errorToResp.HandleJSON(
		func(p httprequest.Params) (interface{}, error) {
			return h(p.Response.Header(), p.Request)
		},
	)
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		handleJSON(w, req, nil)
	})
}

// HandleJSON converts from a ErrorHandler function to an http.Handler.
func HandleErrors(h ErrorHandler) http.Handler {
	// We can't use errorToResp.HandleErrors directly because
	// we still use old-style handlers in charmstore, so we
	// insert shim functions to do the conversion.
	handleErrors := errorToResp.HandleErrors(
		func(p httprequest.Params) error {
			return h(p.Response, p.Request)
		},
	)
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		handleErrors(w, req, nil)
	})
}

var errorToResp httprequest.ErrorMapper = func(err error) (int, interface{}) {
	status, body := errorToResp1(err)
	logger.Infof("error response %d; %s", status, errgo.Details(err))
	return status, body
}

func errorToResp1(err error) (int, interface{}) {
	// Allow bakery errors to be returned as the bakery would
	// like them, so that httpbakery.Client.Do will work.
	if err, ok := errgo.Cause(err).(*httpbakery.Error); ok {
		return httpbakery.ErrorToResponse(err)
	}
	errorBody := errorResponseBody(err)
	status := http.StatusInternalServerError
	switch errorBody.Code {
	case params.ErrNotFound, params.ErrMetadataNotFound:
		status = http.StatusNotFound
	case params.ErrBadRequest, params.ErrInvalidEntity:
		status = http.StatusBadRequest
	case params.ErrForbidden, params.ErrEntityIdNotAllowed:
		status = http.StatusForbidden
	case params.ErrUnauthorized:
		status = http.StatusUnauthorized
	case params.ErrMethodNotAllowed:
		// TODO(rog) from RFC 2616, section 4.7: An Allow header
		// field MUST be present in a 405 (Method Not Allowed)
		// response.
		// Perhaps we should not ever return StatusMethodNotAllowed.
		status = http.StatusMethodNotAllowed
	case params.ErrServiceUnavailable:
		status = http.StatusServiceUnavailable
	}
	return status, errorBody
}

// errorResponse returns an appropriate error
// response for the provided error.
func errorResponseBody(err error) *params.Error {

	errResp := &params.Error{
		Message: err.Error(),
	}
	cause := errgo.Cause(err)
	if coder, ok := cause.(errorCoder); ok {
		errResp.Code = coder.ErrorCode()
	}
	if infoer, ok := cause.(errorInfoer); ok {
		errResp.Info = infoer.ErrorInfo()
	}
	return errResp
}

type errorInfoer interface {
	ErrorInfo() map[string]*params.Error
}

type errorCoder interface {
	ErrorCode() params.ErrorCode
}

// multiError holds multiple errors.
type multiError map[string]error

func (err multiError) Error() string {
	return fmt.Sprintf("multiple (%d) errors", len(err))
}

func (err multiError) ErrorCode() params.ErrorCode {
	return params.ErrMultipleErrors
}

func (err multiError) ErrorInfo() map[string]*params.Error {
	m := make(map[string]*params.Error)
	for key, err := range err {
		m[key] = errorResponseBody(err)
	}
	return m
}

// NotFoundHandler is like http.NotFoundHandler except it
// returns a JSON error response.
func NotFoundHandler() http.Handler {
	return HandleErrors(func(w http.ResponseWriter, req *http.Request) error {
		return errgo.WithCausef(nil, params.ErrNotFound, params.ErrNotFound.Error())
	})
}

func NewServeMux() *ServeMux {
	return &ServeMux{http.NewServeMux()}
}

// ServeMux is like http.ServeMux but returns
// JSON errors when pages are not found.
type ServeMux struct {
	*http.ServeMux
}

func (mux *ServeMux) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.RequestURI == "*" {
		mux.ServeMux.ServeHTTP(w, req)
		return
	}
	h, pattern := mux.Handler(req)
	if pattern == "" {
		WriteError(w, errgo.WithCausef(nil, params.ErrNotFound, "no handler for %q", req.URL.Path))
		return
	}
	h.ServeHTTP(w, req)
}

// RelativeURLPath returns a relative URL path that is lexically
// equivalent to targpath when interpreted by url.URL.ResolveReference.
// On success, the returned path will always be non-empty and relative
// to basePath, even if basePath and targPath share no elements.
//
// An error is returned if basePath or targPath are not absolute paths.
func RelativeURLPath(basePath, targPath string) (string, error) {
	if !strings.HasPrefix(basePath, "/") {
		return "", errgo.Newf("non-absolute base URL")
	}
	if !strings.HasPrefix(targPath, "/") {
		return "", errgo.Newf("non-absolute target URL")
	}
	baseParts := strings.Split(basePath, "/")
	targParts := strings.Split(targPath, "/")

	// For the purposes of dotdot, the last element of
	// the paths are irrelevant. We save the last part
	// of the target path for later.
	lastElem := targParts[len(targParts)-1]
	baseParts = baseParts[0 : len(baseParts)-1]
	targParts = targParts[0 : len(targParts)-1]

	// Find the common prefix between the two paths:
	var i int
	for ; i < len(baseParts); i++ {
		if i >= len(targParts) || baseParts[i] != targParts[i] {
			break
		}
	}
	dotdotCount := len(baseParts) - i
	targOnly := targParts[i:]
	result := make([]string, 0, dotdotCount+len(targOnly)+1)
	for i := 0; i < dotdotCount; i++ {
		result = append(result, "..")
	}
	result = append(result, targOnly...)
	result = append(result, lastElem)
	final := strings.Join(result, "/")
	if final == "" {
		// If the final result is empty, the last element must
		// have been empty, so the target was slash terminated
		// and there were no previous elements, so "."
		// is appropriate.
		final = "."
	}
	return final, nil
}

// TODO(mhilton) This is not an ideal place for UnmarshalJSONResponse,
// maybe it should be in httprequest somewhere?

// UnmarshalJSONResponse unmarshals resp.Body into v. If errorF is not
// nil and resp.StatusCode indicates an error has occured (>= 400) then
// the result of calling errorF with resp is returned.
func UnmarshalJSONResponse(resp *http.Response, v interface{}, errorF func(*http.Response) error) error {
	if errorF != nil && resp.StatusCode >= http.StatusBadRequest {
		return errgo.Mask(errorF(resp), errgo.Any)
	}
	mt, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		return errgo.Notef(err, "cannot parse content type")
	}
	if mt != "application/json" {
		return errgo.Newf("unexpected content type %q", mt)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return errgo.Notef(err, "cannot read response body")
	}
	if err := json.Unmarshal(body, v); err != nil {
		return errgo.Notef(err, "cannot unmarshal response")
	}
	return nil
}
