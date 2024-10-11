// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

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

// Block describes a Juju block that protects model from
// corruption.
type Block struct {
	// Id is this blocks id.
	Id string `json:"id"`

	// Tag holds the tag of the entity that is blocked.
	Tag string `json:"tag"`

	// Type is block type as per model.BlockType.
	// Valid types are "BlockDestroy", "BlockRemove" and "BlockChange".
	Type string `json:"type"`

	// Message is a descriptive or an explanatory message
	// that the block was created with.
	Message string `json:"message,omitempty"`
}

// BlockSwitchParams holds the parameters for switching
// a block on/off.
type BlockSwitchParams struct {
	// Type is block type as per model.BlockType.
	// Valid types are "BlockDestroy", "BlockRemove" and "BlockChange".
	Type string `json:"type"`

	// Message is a descriptive or an explanatory message
	// that accompanies the switch.
	Message string `json:"message,omitempty"`
}

// BlockResult holds the result of an API call to retrieve details
// for a block.
type BlockResult struct {
	Result Block  `json:"result"`
	Error  *Error `json:"error,omitempty"`
}

// BlockResults holds the result of an API call to list blocks.
type BlockResults struct {
	Results []BlockResult `json:"results,omitempty"`
}
