// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package configstore

import (
	"fmt"

	"github.com/juju/errors"
)

var ErrEnvironInfoAlreadyExists = errors.New("model info already exists")

// Storage stores environment and server configuration data.
type Storage interface {
	// ReadInfo reads information associated with
	// the environment with the given name.
	// If there is no such information, it will
	// return an errors.NotFound error.
	ReadInfo(envName string) (EnvironInfo, error)

	// CreateInfo creates some uninitialized information associated
	// with the environment with the given name.
	CreateInfo(controllerUIID, envName string) EnvironInfo
}

// EnvironInfoName returns a name suitable for use in the Storage.CreateInfo
// and ReadInfo methods.
func EnvironInfoName(controller, model string) string {
	return fmt.Sprintf("%s:%s", controller, model)
}

// AdminModelName returns the name of the admin model for a given controller.
//
// NOTE(axw) when configstore is gone, we'll move this to the bootstrap
// command code.
const AdminModelName = "admin"

// EnvironInfo holds information associated with an environment.
type EnvironInfo interface {
	// Initialized returns whether the environment information has
	// been initialized. It will return true for EnvironInfo instances
	// that have been created but not written.
	Initialized() bool

	// BootstrapConfig returns the configuration attributes
	// that an environment will be bootstrapped with.
	BootstrapConfig() map[string]interface{}

	// SetBootstrapConfig sets the configuration attributes
	// to be used for bootstrapping.
	// This method may only be called on an EnvironInfo
	// obtained using ConfigStorage.CreateInfo.
	SetBootstrapConfig(map[string]interface{})

	// Location returns the location of the source of the environment
	// information in a human readable format.
	Location() string

	// Write writes the current information to persistent storage. A
	// subsequent call to ConfigStorage.ReadInfo can retrieve it. After this
	// call succeeds, Initialized will return true.
	// It return ErrAlreadyExists if the EnvironInfo is not yet Initialized
	// and the EnvironInfo has been written before.
	Write() error

	// Destroy destroys the information associated with
	// the environment.
	Destroy() error
}
