// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package export

import (
	"github.com/juju/juju/core/semversion"
)

// ModelExport contains a typed model export payload and the export version.
// Use the RPC wire export envelope when serializing across JSON or YAML
// boundaries.
type ModelExport struct {
	// Version is the Juju semantic version that generated this export. It is
	// typed (not a string) so it has Compare/equality semantics and a single
	// source of truth across the codebase. The wire format is unchanged:
	// semversion.Number marshals to the canonical "4.0.6"-style string in both
	// JSON and YAML.
	Version semversion.Number `json:"version" yaml:"version"`

	// Payload is export struct specific to the version,
	// populated with a model's data.
	Payload any `json:"payload" yaml:"payload"`
}
