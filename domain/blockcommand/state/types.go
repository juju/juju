// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

type blockCommand struct {
	UUID      string `db:"uuid"`
	BlockType int8   `db:"block_command_type_id"`
	Message   string `db:"message"`
}

type blockType struct {
	ID int8 `db:"id"`
}
