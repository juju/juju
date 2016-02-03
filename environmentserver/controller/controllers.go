// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package controller provides functionality to parse information
// describing controllers.

package controller

import (
	"github.com/juju/errors"
)

// Controllers is a struct containing controllers definitions.
type Controllers struct {
	// Controllers is a map of controllers definitions,
	// keyed on controller name.
	Controllers map[string]Controller `yaml:"controllers"`
}

// Controller is a controller definition.
type Controller struct {
	// Servers is the collection of host names running in this controller.
	Servers []string `yaml:"servers,flow"`

	// ControllerUUID is controller unique ID.
	ControllerUUID string `yaml:"uuid"`

	// APIEndpoints is the collection of API endpoints running in this controller.
	APIEndpoints []string `yaml:"api-endpoints,flow"`

	// CACert is a security certificate for this controller.
	CACert string `yaml:"ca-cert"`
}

// ControllerMetadata gets all controllers defined in the controllers file.
func ControllerMetadata() (map[string]Controller, error) {
	return ReadControllersFile(JujuControllersPath())
}

// ControllerByName returns the controller with the specified name.
// If there exists no controller with the specified name, an
// error satisfying errors.IsNotFound will be returned.
func ControllerByName(name string) (*Controller, error) {
	controllers, err := ControllerMetadata()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if result, ok := controllers[name]; ok {
		return &result, nil
	}
	return nil, errors.NotFoundf("controller %s", name)
}

// UpdateController adds given controller to the controllers.
// If controller does not exist in the given data, it will be added.
// If controller exists, it will be overwritten with new values.
// This assumes that there are no 2 controllers with the same name.
// Once controllers collection is updated, controllers file is written.
func UpdateController(name string, one Controller) error {
	all, err := ControllerMetadata()
	if err != nil {
		return errors.Annotate(err, "cannot get controllers")
	}

	if len(all) == 0 {
		all = make(map[string]Controller)
	}

	all[name] = one
	return WriteControllersFile(&Controllers{all})
}

// RemoveController removes controller with the given name from the controllers
// collection.
// Once controllers collection is updated, controllers file is written.
func RemoveController(name string) error {
	all, err := ControllerMetadata()
	if err != nil {
		return errors.Annotate(err, "cannot get controllers")
	}

	delete(all, name)
	return WriteControllersFile(&Controllers{all})
}
