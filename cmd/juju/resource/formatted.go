// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import "time"

// FormattedCharmResource holds the formatted representation of a resource's info.
type FormattedCharmResource struct {
	// These fields are exported for the sake of serialization.
	Name        string `json:"name" yaml:"name"`
	Type        string `json:"type" yaml:"type"`
	Path        string `json:"path" yaml:"path"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Revision    int    `json:"revision,omitempty" yaml:"revision,omitempty"`
	Fingerprint string `json:"fingerprint" yaml:"fingerprint"`
	Size        int64  `json:"size" yaml:"size"`
	Origin      string `json:"origin" yaml:"origin"`
}

// FormattedApplicationInfo holds the formatted representation of the information
// about an application's resources.
type FormattedApplicationInfo struct {
	Resources []FormattedAppResource   `json:"resources,omitempty" yaml:"resources,omitempty"`
	Updates   []FormattedCharmResource `json:"updates,omitempty" yaml:"updates,omitempty"`
}

// FormattedAppResource holds the formatted representation of a resource's info.
type FormattedAppResource struct {
	// These fields are exported for the sake of serialization.
	ID            string    `json:"resourceid,omitempty" yaml:"resourceid,omitempty"`
	ApplicationID string    `json:"applicationId,omitempty" yaml:"applicationId,omitempty"`
	Name          string    `json:"name" yaml:"name"`
	Type          string    `json:"type" yaml:"type"`
	Path          string    `json:"path" yaml:"path"`
	Description   string    `json:"description,omitempty" yaml:"description,omitempty"`
	Revision      string    `json:"revision,omitempty" yaml:"revision,omitempty"`
	Fingerprint   string    `json:"fingerprint" yaml:"fingerprint"`
	Size          int64     `json:"size" yaml:"size"`
	Origin        string    `json:"origin" yaml:"origin"`
	Used          bool      `json:"used" yaml:"used"`
	Timestamp     time.Time `json:"timestamp,omitempty" yaml:"timestamp,omitempty"`
	Username      string    `json:"username,omitempty" yaml:"username,omitempty"`

	CombinedRevision string `json:"-"`
	UsedYesNo        string `json:"-"`
	CombinedOrigin   string `json:"-"`
}

// FormattedDetailResource is the data for a single line of tabular output for
// juju resources <application> --details.
type FormattedDetailResource struct {
	UnitID      string               `json:"unitID" yaml:"unitID"`
	Unit        FormattedAppResource `json:"unit" yaml:"unit"`
	Expected    FormattedAppResource `json:"expected" yaml:"expected"`
	Progress    int64                `json:"progress,omitempty" yaml:"progress,omitempty"`
	UnitNumber  int                  `json:"-"`
	RevProgress string               `json:"-"`
}

// FormattedApplicationDetails is the data for the tabular output for juju resources
// <application> --details.
type FormattedApplicationDetails struct {
	Resources []FormattedDetailResource `json:"resources,omitempty" yaml:"resources,omitempty"`
	Updates   []FormattedCharmResource  `json:"updates,omitempty" yaml:"updates,omitempty"`
}

// FormattedDetailResource is the data for the tabular output for juju resources
// <unit> --details.
type FormattedUnitDetails []FormattedDetailResource
