// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

import (
	"time"
)

// UserEnvironment holds information about an environment and the last
// time the environment was accessed for a particular user. This is a client
// side structure that translates the owner tag into a user facing string.
type UserEnvironment struct {
	Name           string
	UUID           string
	Owner          string
	LastConnection *time.Time
}
