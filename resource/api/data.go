// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

// TODO(ericsnow) Eliminate the dependence on apiserver if possible.

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
)

// ListResourcesArgs are the arguments for the ListResources endpoint.
type ListResourcesArgs params.Entities

// NewListResourcesArgs returns the arguments for the ListResources endpoint.
func NewListResourcesArgs(services []string) (ListResourcesArgs, error) {
	var args ListResourcesArgs
	for _, service := range services {
		if !names.IsValidService(service) {
			return args, errors.Errorf("invalid service %q", service)
		}

		args.Entities = append(args.Entities, params.Entity{
			Tag: names.NewServiceTag(service).String(),
		})
	}
	return args, nil
}

// ResourcesResults holds the resources that result
// from a bulk API call.
type ResourcesResults struct {
	// Results is the list of resource results.
	Results []ResourcesResult
}

// ResourcesResult holds the resources that result from an API call
// for a single service.
type ResourcesResult struct {
	params.ErrorResult

	// Resources is the list of resources for the service.
	Resources []Resource
}

// NewResourcesResult produces a ResourcesResult for the given service
// tag. The corresponding service ID is also returned. If any error
// results, it is stored in the Error field of the result.
func NewResourcesResult(tagStr string) (ResourcesResult, string) {
	var result ResourcesResult

	serviceID, err := ServiceTag2ID(tagStr)
	if err != nil {
		result.Error = &params.Error{
			Message: err.Error(),
			Code:    params.CodeBadRequest,
		}
		return result, ""
	}

	return result, serviceID
}

// SetResultError sets the error on the result.
func SetResultError(result *ResourcesResult, err error) {
	result.Error = common.ServerError(err)
}

// Resource contains info about a Resource.
type Resource struct {
	CharmResource

	// Charm identifies the resource's charm.
	Charm names.CharmTag

	// Username is the ID of the user that added the revision
	// to the model (whether implicitly or explicitly).
	Username string `json:"username"`

	// Timestamp indicates when the resource was added to the model.
	Timestamp time.Time `json:"timestamp"`
}

// CharmResource contains the definition for a resource.
type CharmResource struct {
	// Name identifies the resource.
	Name string `json:"name"`

	// Type is the name of the resource type.
	Type string `json:"type"`

	// Path is where the resource will be stored.
	Path string `json:"path"`

	// Comment contains user-facing info about the resource.
	Comment string `json:"comment,omitempty"`

	// Origin is where the resource will come from.
	Origin string `json:"origin"`

	// Revision is the revision, if applicable.
	Revision int `json:"revision"`

	// Fingerprint is the SHA-384 checksum for the resource blob.
	Fingerprint []byte `json:"fingerprint"`
}
