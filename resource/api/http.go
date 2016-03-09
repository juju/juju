// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

// TODO(ericsnow) Eliminate the apiserver dependencies, if possible.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
)

var logger = loggo.GetLogger("juju.resource.api")

const (
	// HTTPEndpointPattern is the URL path pattern registered with
	// the API server. This includes wildcards (starting with ":") that
	// are converted into URL query values by the pattern mux. Also see
	// apiserver/apiserver.go.
	HTTPEndpointPattern = "/services/:service/resources/:resource"

	// HTTPEndpointPath is the URL path, with substitutions, for
	// a resource request.
	HTTPEndpointPath = "/services/%s/resources/%s"
)

const (
	// ContentTypeRaw is the HTTP content-type value used for raw, unformattedcontent.
	ContentTypeRaw = "application/octet-stream"

	// ContentTypeJSON is the HTTP content-type value used for JSON content.
	ContentTypeJSON = "application/json"
)

// NewEndpointPath returns the API URL path for the identified resource.
func NewEndpointPath(service, name string) string {
	return fmt.Sprintf(HTTPEndpointPath, service, name)
}

// ExtractEndpointDetails pulls the endpoint wildcard values from
// the provided URL.
func ExtractEndpointDetails(url *url.URL) (service, name string) {
	service = url.Query().Get(":service")
	name = url.Query().Get(":resource")
	return service, name
}

// TODO(ericsnow) These are copied from apiserver/httpcontext.go...

// SendHTTPError sends a JSON-encoded error response
// for errors encountered during processing.
func SendHTTPError(w http.ResponseWriter, err error) {
	err1, statusCode := common.ServerErrorAndStatus(err)
	logger.Debugf("sending error: %d %v", statusCode, err1)
	SendHTTPStatusAndJSON(w, statusCode, &params.ErrorResult{
		Error: err1,
	})
}

// SendStatusAndJSON sends an HTTP status code and
// a JSON-encoded response to a client.
func SendHTTPStatusAndJSON(w http.ResponseWriter, statusCode int, response interface{}) {
	body, err := json.Marshal(response)
	if err != nil {
		http.Error(w, errors.Annotatef(err, "cannot marshal JSON result %#v", response).Error(), 504)
		return
	}

	if statusCode == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", `Basic realm="juju"`)
	}
	w.Header().Set("Content-Type", params.ContentTypeJSON)
	w.Header().Set("Content-Length", fmt.Sprint(len(body)))
	w.WriteHeader(statusCode)
	w.Write(body)
}
