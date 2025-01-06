// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package architecture

// Architecture represents an application's architecture.
type Architecture int

const (
	Unknown Architecture = iota - 1

	AMD64
	ARM64
	PPC64EL
	S390X
	RISCV64
)
