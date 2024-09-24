// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// modelCloudType represents 
type modelCloudType struct {
	CloudType string `db:"cloud_type"`
}

// modelUUIDValue represents a model id for associating public keys with.
type modelUUIDValue struct {
	UUID string `db:"model_uuid"`
}
