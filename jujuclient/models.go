// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient

import (
	"io/ioutil"
	"os"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/juju/osenv"
)

// JujuModelsPath is the location where models information is
// expected to be found.
func JujuModelsPath() string {
	// TODO(axw) models.yaml should go into XDG_CACHE_HOME.
	return osenv.JujuXDGDataHomePath("models.yaml")
}

// ReadModelsFile loads all models defined in a given file.
// If the file is not found, it is not an error.
func ReadModelsFile(file string) (map[string]ControllerAccountModels, error) {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	models, err := ParseModels(data)
	if err != nil {
		return nil, err
	}
	return models, nil
}

// WriteModelsFile marshals to YAML details of the given models
// and writes it to the models file.
func WriteModelsFile(models map[string]ControllerAccountModels) error {
	data, err := yaml.Marshal(modelsCollection{models})
	if err != nil {
		return errors.Annotate(err, "cannot marshal models")
	}
	return utils.AtomicWriteFile(JujuModelsPath(), data, os.FileMode(0600))
}

// ParseModels parses the given YAML bytes into models metadata.
func ParseModels(data []byte) (map[string]ControllerAccountModels, error) {
	var result modelsCollection
	err := yaml.Unmarshal(data, &result)
	if err != nil {
		return nil, errors.Annotate(err, "cannot unmarshal models")
	}
	return result.ControllerAccountModels, nil
}

type modelsCollection struct {
	ControllerAccountModels map[string]ControllerAccountModels `yaml:"controllers"`
}

// ControllerAccountModels stores per-controller account-model information.
type ControllerAccountModels struct {
	// AccountModels is the collection of account-models for the
	// controller.
	AccountModels map[string]*AccountModels `yaml:"accounts"`
}

// AccountModels stores per-account model information.
type AccountModels struct {
	// Models is the collection of models for the account. This should
	// be treated as a cache only, and not the complete set of models
	// for the account.
	Models map[string]ModelDetails `yaml:"models"`

	// CurrentModel is the name of the active model for the account.
	CurrentModel string `yaml:"current-model,omitempty"`
}
