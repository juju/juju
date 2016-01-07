// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import "time"

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

	// These fields are not exported so they won't be serialized, since they are
	// specific to the tabular output.
	charmRevision string
}

// FormattedSvcResource holds the formatted representation of a resource's info.
type FormattedSvcResource struct {
	// These fields are exported for the sake of serialization.
	Name        string    `json:"name" yaml:"name"`
	Type        string    `json:"type" yaml:"type"`
	Path        string    `json:"path" yaml:"path"`
	Comment     string    `json:"comment,omitempty" yaml:"comment,omitempty"`
	Revision    int       `json:"revision,omitempty" yaml:"revision,omitempty"`
	Fingerprint string    `json:"fingerprint" yaml:"fingerprint"`
	Origin      string    `json:"origin" yaml:"origin"`
	Used        bool      `json:"used" yaml:"used"`
	Timestamp   time.Time `json:"timestamp" yaml:"timestamp"`
	Username    string    `json:"username" yaml:"username"`

	// These fields are not exported so they won't be serialized, since they are
	// specific to the tabular output.
	combinedRevision string
	usedYesNo        string
	combinedOrigin   string
}
