// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

// BlockType values define model block type, which can be used to prevent
// accidental damage to Juju deployments.
type BlockType = string

const (
	// BlockDestroy type identifies destroy blocks.
	BlockDestroy BlockType = "BlockDestroy"

	// BlockRemove type identifies remove blocks.
	BlockRemove BlockType = "BlockRemove"

	// BlockChange type identifies change blocks.
	BlockChange BlockType = "BlockChange"
)
