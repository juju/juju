// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// ListResourcesArgs holds the arguments for an API request to list
// resources for an application. The application is implicit to the uniter-
// specific HTTP connection.
type ListUnitResourcesArgs struct {
	// ResourceNames holds the names of the application's resources for
	// which information should be provided.
	ResourceNames []string `json:"resource-names"`
}

// UnitResourcesResult holds the resource info for a list of requested
// resources.
type UnitResourcesResult struct {
	ErrorResult

	// Resources is the list of results for the requested resources,
	// in the same order as requested.
	Resources []UnitResourceResult `json:"resources"`
}

// UnitResourceResult is the result for a single requested resource.
type UnitResourceResult struct {
	ErrorResult

	// Resource is the info for the requested resource.
	Resource Resource `json:"resource"`
}
