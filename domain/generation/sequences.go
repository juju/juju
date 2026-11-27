// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package generation

import "github.com/juju/juju/domain/sequence"

const (
	// GenerationSequenceNamespace is the sequence namespace used to
	// allocate human facing generation identifiers from the sequence
	// table.
	GenerationSequenceNamespace = sequence.StaticNamespace("generation")
)
