// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// DestroyControllerArgs holds the arguments for destroying a controller.
type DestroyControllerArgs struct {
	// DestroyModels specifies whether or not the hosted models
	// should be destroyed as well. If this is not specified, and there are
	// other hosted models, the destruction of the controller will fail.
	DestroyModels bool `json:"destroy-models"`
}

// ModelBlockInfo holds information about an model and its
// current blocks.
type ModelBlockInfo struct {
	Name     string   `json:"name"`
	UUID     string   `json:"model-uuid"`
	OwnerTag string   `json:"owner-tag"`
	Blocks   []string `json:"blocks"`
}

// ModelBlockInfoList holds information about the blocked models
// for a controller.
type ModelBlockInfoList struct {
	Models []ModelBlockInfo `json:"models,omitempty"`
}

// RemoveBlocksArgs holds the arguments for the RemoveBlocks command. It is a
// struct to facilitate the easy addition of being able to remove blocks for
// individual models at a later date.
type RemoveBlocksArgs struct {
	All bool `json:"all"`
}

// ModelStatus holds information about the status of a juju model.
type ModelStatus struct {
	ModelTag           string `json:"model-tag"`
	Life               Life   `json:"life"`
	HostedMachineCount int    `json:"hosted-machine-count"`
	ServiceCount       int    `json:"service-count"`
	OwnerTag           string `json:"owner-tag"`
}

// ModelStatusResults holds status information about a group of models.
type ModelStatusResults struct {
	Results []ModelStatus `json:"models"`
}

// InitiateModelMigrationArgs holds the details required to start one
// or more model migrations.
type InitiateModelMigrationArgs struct {
	Specs []ModelMigrationSpec `json:"specs"`
}

// ModelMigrationSpec holds the details required to start the
// migration of a single model.
type ModelMigrationSpec struct {
	ModelTag   string                   `json:"model-tag"`
	TargetInfo ModelMigrationTargetInfo `json:"target-info"`
}

// ModelMigrationTargetInfo holds the details required to connect to
// and authenticate with a remote controller for model migration.
type ModelMigrationTargetInfo struct {
	ControllerTag string   `json:"controller-tag"`
	Addrs         []string `json:"addrs"`
	CACert        string   `json:"ca-cert"`
	AuthTag       string   `json:"auth-tag"`
	Password      string   `json:"password"`
}

// InitiateModelMigrationResults is used to return the result of one
// or more attempts to start model migrations.
type InitiateModelMigrationResults struct {
	Results []InitiateModelMigrationResult `json:"results"`
}

// InitiateModelMigrationResult is used to return the result of one
// model migration initiation attempt.
type InitiateModelMigrationResult struct {
	ModelTag string `json:"model-tag"`
	Error    *Error `json:"error"`
	Id       string `json:"id"` // the ID for the migration attempt
}
