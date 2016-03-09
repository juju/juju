// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/httprequest"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/juju/apiserver/params"
)

// HTTPClient implements Connection.APICaller.HTTPClient.
func (s *state) HTTPClient() (*httprequest.Client, error) {
	if !s.isLoggedIn() {
		return nil, errors.New("no HTTP client available without logging in")
	}
	baseURL, err := s.apiEndpoint("/", "")
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &httprequest.Client{
		BaseURL: baseURL.String(),
		Doer: httpRequestDoer{
			st: s,
		},
		UnmarshalError: unmarshalHTTPErrorResponse,
	}, nil
}

// httpRequestDoer implements httprequest.Doer and httprequest.DoerWithBody
// by using httpbakery and the state to make authenticated requests to
// the API server.
type httpRequestDoer struct {
	st *state
}

var _ httprequest.Doer = httpRequestDoer{}

var _ httprequest.DoerWithBody = httpRequestDoer{}

// Do implements httprequest.Doer.Do.
func (doer httpRequestDoer) Do(req *http.Request) (*http.Response, error) {
	return doer.DoWithBody(req, nil)
}

// DoWithBody implements httprequest.DoerWithBody.DoWithBody.
func (doer httpRequestDoer) DoWithBody(req *http.Request, body io.ReadSeeker) (*http.Response, error) {
	// Add basic auth if appropriate
	// Call doer.bakeryClient.DoWithBodyAndCustomError
	if doer.st.tag != "" {
		req.SetBasicAuth(doer.st.tag, doer.st.password)
	}
	return doer.st.bakeryClient.DoWithBodyAndCustomError(req, body, func(resp *http.Response) error {
		// At this point we are only interested in errors that
		// the bakery cares about, and the CodeDischargeRequired
		// error is the only one, and that always comes with a
		// response code StatusUnauthorized.
		if resp.StatusCode != http.StatusUnauthorized {
			return nil
		}
		return bakeryError(unmarshalHTTPErrorResponse(resp))
	})
}

// unmarshalHTTPErrorResponse unmarshals an error response from
// an HTTP endpoint. For historical reasons, these endpoints
// return several different incompatible error response formats.
// We cope with this by accepting all of the possible formats
// and unmarshaling accordingly.
//
// It always returns a non-nil error.
func unmarshalHTTPErrorResponse(resp *http.Response) error {
	var body json.RawMessage
	if err := httprequest.UnmarshalJSONResponse(resp, &body); err != nil {
		return errors.Trace(err)
	}
	// genericErrorResponse defines a struct that is compatible with all the
	// known error types, so that we can know which of the
	// possible error types has been returned.
	//
	// Another possible approach might be to look at resp.Request.URL.Path
	// and determine the expected error type from that, but that
	// seems more fragile than this approach.
	type genericErrorResponse struct {
		Error json.RawMessage
	}
	var generic genericErrorResponse
	if err := json.Unmarshal(body, &generic); err != nil {
		return errors.Annotatef(err, "incompatible error response")
	}
	if bytes.HasPrefix(generic.Error, []byte(`"`)) {
		// The error message is in a string, which means that
		// the error must be in a params.CharmsResponse
		var resp params.CharmsResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return errors.Annotatef(err, "incompatible error response")
		}
		return &params.Error{
			Message: resp.Error,
			Code:    resp.ErrorCode,
			Info:    resp.ErrorInfo,
		}
	}
	var errorBody []byte
	if len(generic.Error) > 0 {
		// We have an Error field, therefore the error must be in that.
		// (it's a params.ErrorResponse)
		errorBody = generic.Error
	} else {
		// There wasn't an Error field, so the error must be directly
		// in the body of the response.
		errorBody = body
	}
	var perr params.Error
	if err := json.Unmarshal(errorBody, &perr); err != nil {
		return errors.Annotatef(err, "incompatible error response")
	}
	if perr.Message == "" {
		return errors.Errorf("error response with no message")
	}
	return &perr
}

// bakeryError translates any discharge-required error into
// an error value that the httpbakery package will recognize.
// Other errors are returned unchanged.
func bakeryError(err error) error {
	if params.ErrCode(err) != params.CodeDischargeRequired {
		return err
	}
	errResp := errors.Cause(err).(*params.Error)
	if errResp.Info == nil {
		return errors.Annotatef(err, "no error info found in discharge-required response error")
	}
	// It's a discharge-required error, so make an appropriate httpbakery
	// error from it.
	return &httpbakery.Error{
		Message: err.Error(),
		Code:    httpbakery.ErrDischargeRequired,
		Info: &httpbakery.ErrorInfo{
			Macaroon:     errResp.Info.Macaroon,
			MacaroonPath: errResp.Info.MacaroonPath,
		},
	}
}
