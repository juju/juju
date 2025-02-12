// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// SendError sends a JSON-encoded error response
// for errors encountered during processing.
func SendError(w http.ResponseWriter, errToSend error, logger logger.Logger) error {
	paramsErr, statusCode := apiservererrors.ServerErrorAndStatus(errToSend)
	logger.Debugf(context.TODO(), "sending error: %d %v", statusCode, paramsErr)
	return errors.Capture(SendStatusAndJSON(w, statusCode, &params.ErrorResult{
		Error: paramsErr,
	}))
}

// SendStatusAndJSON sends an HTTP status code and
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
		return errors.Errorf("cannot write response: %w", err)
	}
	return nil
}
