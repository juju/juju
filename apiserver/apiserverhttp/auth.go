// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserverhttp

import "gopkg.in/juju/names.v2"

// Auth is returned by Mux.Authenticate
type Auth struct {
	// Tag returns the tag of the authenticated entity.
	Tag names.Tag

	// Controller reports whether or not the authenticated
	// entity is a controller agent.
	Controller bool
}
