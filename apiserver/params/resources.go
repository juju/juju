// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"time"
)

// ResourceParams describe a resource.
type ResourceParams struct {
	Type     string `json:"type"`
	User     string `json:"user"`
	Org      string `json:"org"`
	Stream   string `json:"stream"`
	Series   string `json:"series"`
	PathName string `json:"pathname"`
	Revision string `json:"revision"`
}

// ResourceFilterParams holds the parameters used to specify resources to list or delete.
type ResourceFilterParams struct {
	Resources []ResourceParams `json:"resources"`
}

// ListResourcesResult holds the results of querying resources.
type ListResourcesResult struct {
	Resources []ResourceMetadata `json:"result"`
}

// ResourceMetadata represents an resource in storage.
type ResourceMetadata struct {
	ResourcePath string    `json:"resourcepath"`
	URL          string    `json:"url"`
	SHA384       string    `json:"sha384"`
	Size         int64     `json:"size"`
	Created      time.Time `json:"created"`
}

// ResourceResult is returned when uploading a resource.
type ResourceResult struct {
	Resource ResourceMetadata
	Error    *Error
}
