// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

// FormattedCharmResource holds the formatted representation of a resource's info.
type FormattedCharmResource struct {
	// These fields are exported for the sake of serialization.
	Name        string `json:"name" yaml:"name"`
	Type        string `json:"type" yaml:"type"`
	Path        string `json:"path" yaml:"path"`
	Comment     string `json:"comment,omitempty" yaml:"comment,omitempty"`
	Revision    int    `json:"revision,omitempty" yaml:"revision,omitempty"`
	Fingerprint string `json:"fingerprint" yaml:"fingerprint"`
	Origin      string `json:"origin" yaml:"origin"`
}
