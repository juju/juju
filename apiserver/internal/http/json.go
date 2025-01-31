// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package http

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/juju/errors"

	"github.com/juju/juju/rpc/params"
)

// sendStatusAndJSON sends an HTTP status code and
// a JSON-encoded response to a client.
func SendStatusAndJSON(w http.ResponseWriter, statusCode int, response interface{}) error {
	body, err := json.Marshal(response)
	if err != nil {
		return errors.Errorf("cannot marshal JSON result %#v: %v", response, err)
	}

	if statusCode == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", `Basic realm="juju"`)
	}
	w.Header().Set("Content-Type", params.ContentTypeJSON)
	w.Header().Set("Content-Length", fmt.Sprint(len(body)))
	w.WriteHeader(statusCode)
	if _, err := w.Write(body); err != nil {
		return errors.Annotate(err, "cannot write response")
	}
	return nil
}
