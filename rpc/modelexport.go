// Copyright 2026 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package rpc

// ModelExport is the wire envelope for transporting versioned model exports.
type ModelExport struct {
	// Version denotes the earliest Juju version
	// that this payload is compatible with.
	Version string `json:"version" yaml:"version"`

	// Payload contains the version-specific data from the model database.
	Payload any `json:"payload" yaml:"payload"`
}
