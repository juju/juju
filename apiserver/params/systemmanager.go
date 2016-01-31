// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// DestroyControllerArgs holds the arguments for destroying a controller.
type DestroyControllerArgs struct {
	// DestroyEnvironments specifies whether or not the hosted environments
	// should be destroyed as well. If this is not specified, and there are
	// other hosted environments, the destruction of the controller will fail.
	DestroyEnvironments bool `json:"destroy-environments"`
}

// EnvironmentBlockInfo holds information about an environment and its
// current blocks.
type EnvironmentBlockInfo struct {
	Name     string   `json:"name"`
	UUID     string   `json:"env-uuid"`
	OwnerTag string   `json:"owner-tag"`
	Blocks   []string `json:"blocks"`
}

// EnvironmentBlockInfoList holds information about the blocked environments
// for a controller.
type EnvironmentBlockInfoList struct {
	Environments []EnvironmentBlockInfo `json:"environments,omitempty"`
}

// RemoveBlocksArgs holds the arguments for the RemoveBlocks command. It is a
// struct to facilitate the easy addition of being able to remove blocks for
// individual environments at a later date.
type RemoveBlocksArgs struct {
	All bool `json:"all"`
}

// EnvironmentStatus holds information about the status of a juju environment.
type EnvironmentStatus struct {
	EnvironTag         string `json:"environ-tag"`
	Life               Life   `json:"life"`
	HostedMachineCount int    `json:"hosted-machine-count"`
	ServiceCount       int    `json:"service-count"`
	OwnerTag           string `json:"owner-tag"`
}

// EnvironmentStatusResult holds status information about a group of environments.
type EnvironmentStatusResults struct {
	Results []EnvironmentStatus `json:"environments"`
}
