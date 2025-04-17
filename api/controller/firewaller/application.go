// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"github.com/juju/names/v6"
)

// Application represents the state of an application.
type Application struct {
	client *Client
	tag    names.ApplicationTag
}

// Name returns the application name.
func (s *Application) Name() string {
	return s.tag.Id()
}

// Tag returns the application tag.
func (s *Application) Tag() names.ApplicationTag {
	return s.tag
}
