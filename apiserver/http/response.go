// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package http

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
)

// ExtractJSONResult unserializes the JSON-encoded result into the
// provided struct.
func ExtractJSONResult(resp *http.Response, result interface{}) error {
	// We defer closing the body here because we want it closed whether
	// or not the subsequent read fails.
	defer resp.Body.Close()

	if resp.Header.Get("Content-Type") != CTypeJSON {
		return errors.Errorf(`expected "application/json" content type, got %q`, resp.Header.Get("Content-Type"))
	}

	err := json.NewDecoder(resp.Body).Decode(result)
	return errors.Trace(err)
}

// ExtractAPIError returns the failure serialized in the response
// body.  If there is no failure (an OK status code), it simply returns
// nil.
func ExtractAPIError(resp *http.Response) (*params.Error, error) {
	if resp.StatusCode == http.StatusOK {
		return nil, nil
	}
	// We defer closing the body here because we want it closed whether
	// or not the subsequent read fails.
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Annotate(err, "while reading HTTP response")
	}

	var failure params.Error
	if resp.Header.Get("Content-Type") == CTypeJSON {
		if err := json.Unmarshal(body, &failure); err != nil {
			return nil, errors.Annotate(err, "while unserializing the error")
		}
	} else {
		switch resp.StatusCode {
		case http.StatusNotFound, http.StatusMethodNotAllowed:
			failure.Code = params.CodeNotImplemented
		default:
			// Leave Code empty.
		}

		failure.Message = string(body)
	}
	return &failure, nil
}
