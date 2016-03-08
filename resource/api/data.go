// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

// TODO(ericsnow) Eliminate the dependence on apiserver if possible.

import (
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/apiserver/params"
)

// ListResourcesArgs are the arguments for the ListResources endpoint.
type ListResourcesArgs params.Entities

// NewListResourcesArgs returns the arguments for the ListResources endpoint.
func NewListResourcesArgs(services []string) (ListResourcesArgs, error) {
	var args ListResourcesArgs
	var errs []error
	for _, service := range services {
		if !names.IsValidService(service) {
			err := errors.Errorf("invalid service %q", service)
			errs = append(errs, err)
			continue
		}
		args.Entities = append(args.Entities, params.Entity{
			Tag: names.NewServiceTag(service).String(),
		})
	}
	if err := resolveErrors(errs); err != nil {
		return args, errors.Trace(err)
	}
	return args, nil
}

// AddPendingResourcesArgs holds the arguments to the AddPendingResources
// API endpoint.
type AddPendingResourcesArgs struct {
	params.Entity

	// Resources is the list of resources to add as pending.
	Resources []CharmResource
}

// NewAddPendingResourcesArgs returns the arguments for the
// AddPendingResources API endpoint.
func NewAddPendingResourcesArgs(serviceID string, resources []charmresource.Resource) (AddPendingResourcesArgs, error) {
	var args AddPendingResourcesArgs

	if !names.IsValidService(serviceID) {
		return args, errors.Errorf("invalid service %q", serviceID)
	}
	tag := names.NewServiceTag(serviceID).String()

	var apiResources []CharmResource
	for _, res := range resources {
		if err := res.Validate(); err != nil {
			return args, errors.Trace(err)
		}
		apiRes := CharmResource2API(res)
		apiResources = append(apiResources, apiRes)
	}
	args.Tag = tag
	args.Resources = apiResources
	return args, nil
}

// AddPendingResourcesResult holds the result of the AddPendingResources
// API endpoint.
type AddPendingResourcesResult struct {
	params.ErrorResult

	// PendingIDs holds the "pending ID" for each of the requested
	// resources.
	PendingIDs []string
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

	// CharmStoreResources is the list of resources associated with the charm in
	// the charmstore.
	CharmStoreResources []CharmResource

	// UnitResources contains a list of the resources for each unit in the
	// service.
	UnitResources []UnitResources
}

// A UnitResources contains a list of the resources the unit defined by Entity.
type UnitResources struct {
	params.Entity

	// Resources is a list of resources for the unit.
	Resources []Resource
}

// UploadResult is the response from an upload request.
type UploadResult struct {
	params.ErrorResult

	// Resource describes the resource that was stored in the model.
	Resource Resource
}

// Resource contains info about a Resource.
type Resource struct {
	CharmResource

	// ID uniquely identifies a resource-service pair within the model.
	// Note that the model ignores pending resources (those with a
	// pending ID) except for in a few clearly pending-related places.
	ID string

	// PendingID identifies that this resource is pending and
	// distinguishes it from other pending resources with the same model
	// ID (and from the active resource).
	PendingID string

	// ServiceID identifies the service for the resource.
	ServiceID string

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

	// Description contains user-facing info about the resource.
	Description string `json:"description,omitempty"`

	// Origin is where the resource will come from.
	Origin string `json:"origin"`

	// Revision is the revision, if applicable.
	Revision int `json:"revision"`

	// Fingerprint is the SHA-384 checksum for the resource blob.
	Fingerprint []byte `json:"fingerprint"`

	// Size is the size of the resource, in bytes.
	Size int64
}

func resolveErrors(errs []error) error {
	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errs[0]
	default:
		msgs := make([]string, len(errs))
		for i, err := range errs {
			msgs[i] = err.Error()
		}
		return errors.New(strings.Join(msgs, "\n"))
	}
}
