// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"fmt"
	"net/url"
)

const (
	// HTTPEndpointPattern is the URL path pattern registered with
	// the API server. This includes wildcards (starting with ":") that
	// are converted into URL query values by the pattern mux. Also see
	// apiserver/apiserver.go.
	HTTPEndpointPattern = "/services/:service/resources/:resource"

	// HTTPEndpointPath is the URL path, with substitutions, for
	// a resource request.
	HTTPEndpointPath = "/environment/%s/services/%s/resources/%s"
)

// NewEndpointPath returns the API URL path for the identified resource.
func NewEndpointPath(envUUID, service, name string) string {
	return fmt.Sprintf(HTTPEndpointPath, envUUID, service, name)
}

// ExtractEndpointDetails pulls the endpoint wildcard values from
// the provided URL.
func ExtractEndpointDetails(url *url.URL) (service, name string) {
	service = url.Query().Get(":service")
	name = url.Query().Get(":resource")
	return service, name
}
