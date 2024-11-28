// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package architecture

// Architecture represents an application's architecture.
type Architecture int

const (
	AMD64 Architecture = iota
	ARM64
	PPC64EL
	S390X
	RISV64
)
