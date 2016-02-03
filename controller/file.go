// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package controller provides functionality to parse information
// describing controllers.

package controller

import (
	"io/ioutil"
	"os"

	"github.com/juju/errors"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/juju/osenv"
)

// JujuControllersPath is the location where controllers information is
// expected to be found.
// TODO (anastasiamac 2016-02-02) This needs to be updated to use new and shiny DATA instead of HOME...?
// Requires JUJU_HOME to be set.
func JujuControllersPath() string {
	return osenv.JujuHomePath("controllers.yaml")
}

// ReadControllerMetadataFile loads all controllers defined in a given file.
// If the file is not found, it is not an error.
func ReadControllersFile(file string) (map[string]Controller, error) {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	controllers, err := ParseControllers(data)
	if err != nil {
		return nil, err
	}
	return controllers.Controllers, nil
}

// WriteControllerMetadataFile marshals to YAML details of the given controllers
// and writes it to the controllers file.
func WriteControllersFile(controllers *Controllers) error {
	data, err := yaml.Marshal(controllers)
	if err != nil {
		return errors.Annotate(err, "cannot marshal yaml controllers")
	}
	return ioutil.WriteFile(JujuControllersPath(), data, os.FileMode(0600))
}

// ParseControllerMetadata parses the given YAML bytes into controllers metadata.
func ParseControllers(data []byte) (*Controllers, error) {
	var result Controllers
	err := yaml.Unmarshal(data, &result)
	if err != nil {
		return nil, errors.Annotate(err, "cannot unmarshal yaml controllers metadata")
	}
	return &result, nil
}
