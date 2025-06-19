// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import "github.com/juju/juju/domain/sequence"

const (
	// CharmSequenceNamespace is the namespace for charm sequences.
	CharmSequenceNamespace = sequence.StaticNamespace("charm")

	// ApplicationSequenceNamespace is the namespace for application unit sequences.
	ApplicationSequenceNamespace = sequence.StaticNamespace("application")
)
