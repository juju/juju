// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

// FormattedSpec holds the formatted representation of a resource spec.
type FormattedSpec struct {
	// These fields are exported for the sake of serialization.
	Name     string `json:"name" yaml:"name"`
	Type     string `json:"type" yaml:"type"`
	Path     string `json:"path" yaml:"path"`
	Comment  string `json:"comment,omitempty" yaml:"comment,omitempty"`
	Origin   string `json:"origin" yaml:"origin"`
	Revision string `json:"revision,omitempty" yaml:"revision,omitempty"`
}
