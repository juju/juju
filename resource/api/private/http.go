// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package private

const (
	// HTTPEndpointPattern is the URL path pattern registered with
	// the API server. This includes wildcards (starting with ":") that
	// are converted into URL query values by the pattern mux. Also see
	// apiserver/apiserver.go.
	HTTPEndpointPattern = "/units/:unit/resources/:resource"
)
