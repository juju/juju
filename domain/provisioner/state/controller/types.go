// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

// controllerConfigRow holds a single controller config key-value pair.
type controllerConfigRow struct {
	Key   string `db:"key"`
	Value string `db:"value"`
}

// cloudEndpointRow holds cloud endpoint data from a region query.
type cloudEndpointRow struct {
	Endpoint string `db:"endpoint"`
}

// cloudNameParam is a parameter for cloud name lookups.
type cloudNameParam struct {
	Name string `db:"name"`
}

// cloudRegionNameParam is a parameter for cloud region name lookups.
type cloudRegionNameParam struct {
	RegionName string `db:"region_name"`
}

// imageMetadataRow holds image metadata query results.
type imageMetadataRow struct {
	ImageID         string `db:"image_id"`
	Stream          string `db:"stream"`
	Region          string `db:"region"`
	Version         string `db:"version"`
	Arch            string `db:"arch"`
	VirtType        string `db:"virt_type"`
	RootStorageType string `db:"root_storage_type"`
	RootStorageSize *int64 `db:"root_storage_size"`
	Source          string `db:"source"`
	Priority        *int   `db:"priority"`
}

// imageMetadataFilter holds filter parameters for image metadata queries.
type imageMetadataFilter struct {
	Version string `db:"version"`
	Arch    string `db:"arch"`
	Region  string `db:"region"`
	Stream  string `db:"stream"`
}

// imageMetadataFlags holds boolean flags for conditional filtering.
type imageMetadataFlags struct {
	HasVersion int `db:"has_version"`
	HasArch    int `db:"has_arch"`
	HasRegion  int `db:"has_region"`
	HasStream  int `db:"has_stream"`
}
