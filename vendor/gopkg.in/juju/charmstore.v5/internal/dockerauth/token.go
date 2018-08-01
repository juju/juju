// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dockerauth

const (
	Repository = "repository"
)

// resourceAccessRights specifies the access rights given to a single
// resource.
type resourceAccessRights struct {
	// Type specifies the type of the resource.
	Type string `json:"type"`

	// Name specifies the name of the resource.
	Name string `json:"name"`

	// Actions specifies the allowed actions on the resource.
	Actions []string `json:"actions"`
}
