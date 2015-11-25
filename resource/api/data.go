// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

// ListSpecsArgs are the arguments for the ListSpecs endpoint.
type ListSpecsArgs struct {
	// Service identifies the tag for the service to list.
	Service string
}

// ListSpecsResults holds the results of the ListSpecs endpoint.
type ListSpecsResults struct {
	// Results is the list of resource results.
	Results []ResourceSpec
}

// ResourceSpec contains the definition for a resource.
type ResourceSpec struct {
	// Name identifies the resource.
	Name string

	// Type is the name of the resource type.
	Type string

	// Path is where the resource will be stored.
	Path string

	// Comment contains user-facing info about the resource.
	Comment string

	// Origin is where the resource will come from.
	Origin string

	// Revision is the desired revision, if applicable.
	Revision string
}
