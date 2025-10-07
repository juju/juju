// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package blockcommand

import "github.com/juju/juju/internal/errors"

const (
	// DefaultMaxMessageLength is the default maximum length of a block message.
	DefaultMaxMessageLength = 512
)

// BlockType defines the block type for a command.
type BlockType int8

const (
	// DestroyBlock type identifies block that prevents model destruction.
	DestroyBlock BlockType = 1

	// RemoveBlock type identifies block that prevents
	// removal of machines, applications, units or relations.
	RemoveBlock BlockType = 2

	// ChangeBlock type identifies block that prevents model changes such
	// as additions, modifications, removals of model entities.
	ChangeBlock BlockType = 3
)

// Validate checks if the block type is valid.
func (t BlockType) Validate() error {
	switch t {
	case DestroyBlock, RemoveBlock, ChangeBlock:
		return nil
	}
	return errors.Errorf("invalid block type %d", t)
}

func (t BlockType) String() string {
	switch t {
	case DestroyBlock:
		return "BlockDestroy"
	case RemoveBlock:
		return "BlockRemove"
	case ChangeBlock:
		return "BlockChange"
	}
	return "unknown"
}

// Block represents a command block.
type Block struct {
	UUID    string
	Type    BlockType
	Message string
}
