// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package export

// ModelExport contains a model export payload and the export version.
type ModelExport struct {
	// Version is the Juju semantic version that generated this export.
	Version string

	// Payload is export struct specific to the version,
	// populated with a model's data.
	Payload any
}
