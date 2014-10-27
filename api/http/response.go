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

// ExtractAPIError returns the failure serialized in the response
// body.  If there is no failure (an OK status code), it simply returns
// nil.
func ExtractAPIError(resp *http.Response) (*params.Error, error) {
	if resp.StatusCode == http.StatusOK {
		return nil, nil
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Annotate(err, "while reading HTTP response")
	}

	var failure params.Error
	if resp.Header.Get("Content-Type") == "application/json" {
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
