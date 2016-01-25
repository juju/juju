// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

import (
	"time"

	"github.com/juju/juju/apiserver/params"
)

// UserModel holds information about a model and the last
// time the model was accessed for a particular user. This is a client
// side structure that translates the owner tag into a user facing string.
type UserModel struct {
	Name           string
	UUID           string
	Owner          string
	LastConnection *time.Time
}

// ModelStatus holds information about the status of a juju model.
type ModelStatus struct {
	UUID               string
	Life               params.Life
	Owner              string
	HostedMachineCount int
	ServiceCount       int
}
