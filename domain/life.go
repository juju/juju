// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

// Life represents the life of an entity
// as recorded in the life lookup table.
type Life int

const (
	Alive Life = iota
	Dying
	Dead
)
