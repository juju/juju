// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// These structs represent the cloud and model config schemas in the database.

type attrs []string

type modelUUID struct {
	UUID string `db:"uuid"`
}

type cloudUUIDValue struct {
	UUID string `db:"uuid"`
}

type cloudNameValue struct {
	Name string `db:"name"`
}

type cloudRegion struct {
	UUID      string `db:"uuid"`
	CloudUUID string `db:"cloud_uuid"`
	Name      string `db:"name"`
}

type cloudDefaultValue struct {
	UUID  string `db:"cloud_uuid"`
	Key   string `db:"key"`
	Value string `db:"value"`
}

type keyValue struct {
	Key   string `db:"key"`
	Value string `db:"value"`
}

type cloudRegionDefaultValue struct {
	UUID       string `db:"region_uuid"`
	RegionName string `db:"name"`
	Key        string `db:"key"`
	Value      string `db:"value"`
}

type modelMetadata struct {
	ModelName string `db:"name"`
	CloudType string `db:"type"`
}

// modelCloudType represents the cloud type of the models cloud.
type modelCloudType struct {
	CloudType string `db:"cloud_type"`
}
