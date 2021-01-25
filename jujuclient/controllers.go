// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient

import (
	"io/ioutil"
	"os"

	"github.com/juju/errors"
	"github.com/juju/utils/v2"
	"gopkg.in/yaml.v3"

	"github.com/juju/juju/juju/osenv"
)

// JujuControllersPath is the location where controllers information is
// expected to be found.
func JujuControllersPath() string {
	return osenv.JujuXDGDataHomePath("controllers.yaml")
}

// ReadControllersFile loads all controllers defined in a given file.
// If the file is not found, it is not an error.
func ReadControllersFile(file string) (*Controllers, error) {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			return &Controllers{}, nil
		}
		if os.IsPermission(err) {
			u, userErr := utils.LocalUsername()
			if userErr != nil {
				return nil, err
			}
			if ok, fileErr := utils.IsFileOwner(file, u); fileErr == nil && !ok {
				err = errors.Annotatef(err, "ownership of the file is not the same as the current user")
			}
		}
		return nil, err
	}
	controllers, err := ParseControllers(data)
	if err != nil {
		return nil, err
	}
	return controllers, nil
}

// WriteControllersFile marshals to YAML details of the given controllers
// and writes it to the controllers file.
func WriteControllersFile(controllers *Controllers) error {
	data, err := yaml.Marshal(controllers)
	if err != nil {
		return errors.Annotate(err, "cannot marshal yaml controllers")
	}
	return utils.AtomicWriteFile(JujuControllersPath(), data, os.FileMode(0600))
}

// ParseControllers parses the given YAML bytes into controllers metadata.
func ParseControllers(data []byte) (*Controllers, error) {
	var result Controllers
	err := yaml.Unmarshal(data, &result)
	if err != nil {
		return nil, errors.Annotate(err, "cannot unmarshal yaml controllers metadata")
	}
	return &result, nil
}

// Controllers stores per-client controller information.
type Controllers struct {
	// Controllers is the collection of controllers known to the client.
	Controllers map[string]ControllerDetails `yaml:"controllers"`

	// CurrentController is the name of the active controller.
	CurrentController string `yaml:"current-controller,omitempty"`
}
