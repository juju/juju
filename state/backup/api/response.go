// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"

	"github.com/juju/juju/state/api/params"
)

var filenameRegex = regexp.MustCompile(`attachment; filename="([^"]+)"`)

func parseJSONError(resp *http.Response) (string, error) {
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("could not read HTTP response: %v", err)
	}
	// TODO (ericsnow) Change this to params.Error.
	var jsonResponse params.BackupResponse
	if err := json.Unmarshal(body, &jsonResponse); err != nil {
		return "", fmt.Errorf("could not extract error from HTTP response: %v", err)
	}
	return jsonResponse.Error, nil
}

// CheckAPIResponse checks the HTTP response for an API failure.  This
// involves both the HTTP status code and the response body.  If the
// status code indicates a failure (i.e. not StatusOK) then the response
// body will be consumed and parsed as a JSON serialization of the
// error type used by backup.
func CheckAPIResponse(resp *http.Response) *params.Error {
	var code string

	// Check the status code.
	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusNotFound:
		fallthrough
	case http.StatusMethodNotAllowed:
		code = params.CodeNotImplemented
	default:
		code = ""
	}

	// Handle the error body.
	failure, err := parseJSONError(resp)
	if err != nil {
		failure = fmt.Sprintf("(%v)", err)
	}

	return &params.Error{failure, code}
}

// ExtractFilename returns the filename in the Content-Disposition
// header of the HTTP response, if any.
func ExtractFilename(header *http.Header) (string, error) {
	disp := header.Get("Content-Disposition")
	groups := filenameRegex.FindStringSubmatch(disp)
	if groups == nil {
		return "", fmt.Errorf("no valid header found")
	}
	filename := groups[1]
	return filename, nil
}
