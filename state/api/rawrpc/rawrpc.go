// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rawrpc

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/juju/juju/state/api/params"
)

// HTTPDoer makes an HTTP request. It is implemented by *http.Client,
// for example.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Do uses doer to send an HTTP request and returns the response.
func Do(doer HTTPDoer, req *http.Request) (*http.Response, error) {
	var err error
	// Send the request.
	resp, err := doer.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not send raw request: %v", err)
	}
	if resp.StatusCode == http.StatusOK {
		return resp, nil
	}
	defer resp.Body.Close()

	// Handle API errors.
	switch resp.StatusCode {
	case http.StatusMethodNotAllowed, http.StatusNotFound:
		// Handle a "not implemented" response.  (The API server is too
		// old so the method is not supported.)
		err = &params.Error{
			Message: fmt.Sprintf("API method not supported by server"),
			Code:    params.CodeNotImplemented,
		}
	default:
		var data []byte
		resp.Body.Read(data)
		// Handle any other response as an API error.
		var apiErr params.Error
		err = json.NewDecoder(resp.Body).Decode(&apiErr)
		if err != nil {
			err = fmt.Errorf("could not unpack error response: %v", err)
		} else {
			err = &apiErr
		}
	}
	return nil, err
}
