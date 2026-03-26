// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package export

// ModelExport contains a typed model export payload and the export version.
// Use the RPC wire export envelope when serializing across JSON or YAML
// boundaries.
type ModelExport struct {
	// Version is the Juju semantic version that generated this export.
	Version string `json:"version" yaml:"version"`

	// Payload is export struct specific to the version,
	// populated with a model's data.
	Payload any `json:"payload" yaml:"payload"`
}
