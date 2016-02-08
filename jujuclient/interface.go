// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient

// ControllerDetailsList contains list of controller details.
type ControllerDetailsList struct {
	// Controllers is a map of controllers details,
	// keyed on controller name.
	Controllers map[string]ControllerDetails `yaml:"controllers"`
}

// ControllerDetails holds controller details needed to connect to it.
type ControllerDetails struct {
	// Servers is the collection of host names running in this controller.
	Servers []string `yaml:"servers,flow"`

	// ControllerUUID is controller unique ID.
	ControllerUUID string `yaml:"uuid"`

	// APIEndpoints is the collection of API endpoints running in this controller.
	APIEndpoints []string `yaml:"api-endpoints,flow"`

	// CACert is a security certificate for this controller.
	CACert string `yaml:"ca-cert"`
}

// ControllersUpdater caches controllers.
type ControllersUpdater interface {
	// UpdateController adds given controller to the controllers.
	// If controller does not exist in the given data, it will be added.
	// If controller exists, it will be overwritten with new values.
	// This assumes that there cannot be any 2 controllers with the same name.
	UpdateController(name string, one ControllerDetails) error
}

// ControllersRemover removes controllers.
type ControllersRemover interface {
	// RemoveController removes controller with the given name from the controllers
	// collection.
	RemoveController(name string) error
}

// ControllersGetter gets controllers.
type ControllersGetter interface {
	// AllControllers gets all controllers.
	AllControllers() (map[string]ControllerDetails, error)

	// ControllerByName returns the controller with the specified name.
	// If there exists no controller with the specified name, an
	// error satisfying errors.IsNotFound will be returned.
	ControllerByName(name string) (*ControllerDetails, error)
}

// ControllersCache provides functionality for controllers cache.
type ControllersCache interface {
	ControllersUpdater
	ControllersRemover
	ControllersGetter
}

// Cache defines the methods needed to cache information about
// Juju client.
type Cache interface {
	ControllersCache
}
