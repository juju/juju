// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package private

// TODO(ericsnow) Eliminate the apiserver dependencies, if possible.

import (
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/resource/api"
)

// ListResourcesArgs holds the arguments for an API request to list
// resources for a service. The service is implicit to the uniter-
// specific HTTP connection.
type ListResourcesArgs struct {
	// ResourceNames holds the names of the service's resources for
	// which information should be provided.
	ResourceNames []string
}

// ResourcesResult holds the resource info for a list of requested
// resources.
type ResourcesResult struct {
	params.ErrorResult

	// Resources is the list of results for the requested resources,
	// in the same order as requested.
	Resources []ResourceResult
}

// ResourceResult is the result for a single requested resource.
type ResourceResult struct {
	params.ErrorResult

	// Resource is the info for the requested resource.
	Resource api.Resource
}
