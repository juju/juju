// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rawrpc

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/juju/juju/state/api/params"
)

// ErrorResult is any RPC result that provides an embedded error,
// accessible through the Err() method.
type ErrorResult interface {
	Err() error
}

// TODO(ericsnow) Use an "enum" for error "codes".

// UnpackJSON is used to to extract the raw JSON from the incoming
// stream (data) and unmarshal it into a variable (result).  If result
// implements ErrorResult, any embedded error is returned.
func UnpackJSON(data io.Reader, result interface{}) error {
	if result == nil {
		// Nothing to do.
		return nil
	}

	// Extract the raw JSON from the stream.
	body, err := ioutil.ReadAll(data)
	if err != nil {
		return fmt.Errorf("could not read response data: %v", err)
	}

	// Unpack the raw data into the result variable.
	err = json.Unmarshal(body, result)
	if err != nil {
		return fmt.Errorf("could not unpack response data: %v", err)
	}

	// Extract any error from the result.
	errResult, ok := result.(ErrorResult)
	if ok {
		err = errResult.Err()
		if err != nil {
			return err
			return fmt.Errorf("request failed on server: %v", err)
		}
	}

	return nil
}

type RawClient interface {
	Do(req *http.Request) (*http.Response, error)
}

func Send(client RawClient, req *http.Request, errResult ErrorResult) (*http.Response, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not send raw request: %v", err)
	}

	// Handle a "not implemented" response.
	if resp.StatusCode == http.StatusMethodNotAllowed {
		// API server is too old so the method
		// is not supported; notify the client.
		return nil, &params.Error{
			Message: fmt.Sprintf("method not supported by API server"),
			Code:    params.CodeNotImplemented,
		}
	}

	// Handle a bad response code.
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()

		err = UnpackJSON(resp.Body, errResult)
		if err != nil {
			return nil, err
		}

		return nil, fmt.Errorf("request failed on server (%v)", resp.StatusCode)
	}

	// Success!  Hand off the raw HTTP response for handling.
	return resp, nil
}
