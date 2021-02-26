// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"time"

	"gopkg.in/macaroon.v2"
)

// ListResourcesArgs are the arguments for the ListResources endpoint.
type ListResourcesArgs Entities

// AddPendingResourcesArgs holds the arguments to the AddPendingResources
// API endpoint.
type AddPendingResourcesArgs struct {
	Entity
	AddCharmWithAuthorization

	// Resources is the list of resources to add as pending.
	Resources []CharmResource `json:"resources"`
}

// AddPendingResourcesArgsV2 holds the arguments to the AddPendingResources
// API endpoint.
type AddPendingResourcesArgsV2 struct {
	Entity
	URL                string             `json:"url"`
	CharmOrigin        CharmOrigin        `json:"charm-origin"`
	CharmStoreMacaroon *macaroon.Macaroon `json:"macaroon"`

	// Resources is the list of resources to add as pending.
	Resources []CharmResource `json:"resources"`
}

// AddPendingResourcesResult holds the result of the AddPendingResources
// API endpoint.
type AddPendingResourcesResult struct {
	ErrorResult

	// PendingIDs holds the "pending ID" for each of the requested
	// resources.
	PendingIDs []string `json:"pending-ids"`
}

// ResourcesResults holds the resources that result
// from a bulk API call.
type ResourcesResults struct {
	// Results is the list of resource results.
	Results []ResourcesResult `json:"results"`
}

// ResourcesResult holds the resources that result from an API call
// for a single application.
type ResourcesResult struct {
	ErrorResult

	// Resources is the list of resources for the application.
	Resources []Resource `json:"resources"`

	// CharmStoreResources is the list of resources associated with the charm in
	// the charmstore.
	CharmStoreResources []CharmResource `json:"charm-store-resources"`

	// UnitResources contains a list of the resources for each unit in the
	// application.
	UnitResources []UnitResources `json:"unit-resources"`
}

// A UnitResources contains a list of the resources the unit defined by Entity.
type UnitResources struct {
	Entity

	// Resources is a list of resources for the unit.
	Resources []Resource `json:"resources"`

	// DownloadProgress indicates the number of bytes of a resource file
	// have been downloaded so far the uniter. Only currently downloading
	// resources are included.
	DownloadProgress map[string]int64 `json:"download-progress"`
}

// UploadResult is the response from an upload request.
type UploadResult struct {
	ErrorResult

	// Resource describes the resource that was stored in the model.
	Resource Resource `json:"resource"`
}

// Resource contains info about a Resource.
type Resource struct {
	CharmResource

	// ID uniquely identifies a resource-application pair within the model.
	// Note that the model ignores pending resources (those with a
	// pending ID) except for in a few clearly pending-related places.
	ID string `json:"id"`

	// PendingID identifies that this resource is pending and
	// distinguishes it from other pending resources with the same model
	// ID (and from the active resource).
	PendingID string `json:"pending-id"`

	// ApplicationID identifies the application for the resource.
	ApplicationID string `json:"application"`

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
	Size int64 `json:"size"`
}

// CharmResourcesResults returns a list of charm resource results.
type CharmResourcesResults struct {
	Results [][]CharmResourceResult `json:"results"`
}

// CharmResourceResult returns a charm resource result.
type CharmResourceResult struct {
	ErrorResult
	CharmResource
}
