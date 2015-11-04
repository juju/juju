// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// DestroyControllerArgs holds the arguments for destroying a controller.
type DestroyControllerArgs struct {
	// DestroyEnvironments specifies whether or not the hosted environments
	// should be destroyed as well. If this is not specified, and there are
	// other hosted environments, the destruction of the controller will fail.
	DestroyEnvironments bool `json:"destroy-environments"`

	// IgnoreBlocks specifies whether or not to ignore blocks
	// on hosted environments.
	IgnoreBlocks bool `json:"ignore-blocks"`
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
