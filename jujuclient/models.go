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
func ReadModelsFile(file string) (map[string]*ControllerModels, error) {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if err := migrateLegacyModels(data); err != nil {
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
func WriteModelsFile(models map[string]*ControllerModels) error {
	data, err := yaml.Marshal(modelsCollection{models})
	if err != nil {
		return errors.Annotate(err, "cannot marshal models")
	}
	return utils.AtomicWriteFile(JujuModelsPath(), data, os.FileMode(0600))
}

// ParseModels parses the given YAML bytes into models metadata.
func ParseModels(data []byte) (map[string]*ControllerModels, error) {
	var result modelsCollection
	err := yaml.Unmarshal(data, &result)
	if err != nil {
		return nil, errors.Annotate(err, "cannot unmarshal models")
	}
	return result.ControllerModels, nil
}

type modelsCollection struct {
	ControllerModels map[string]*ControllerModels `yaml:"controllers"`
}

// ControllerModels stores per-controller account-model information.
type ControllerModels struct {
	// Models is the collection of models for the account, indexed
	// by model name. This should be treated as a cache only, and
	// not the complete set of models for the account.
	Models map[string]ModelDetails `yaml:"models,omitempty"`

	// CurrentModel is the name of the active model for the account.
	CurrentModel string `yaml:"current-model,omitempty"`
}

// TODO(axw) 2016-07-14 #NNN
// Drop this code once we get to 2.0-beta13.
func migrateLegacyModels(data []byte) error {
	accounts, err := ReadAccountsFile(JujuAccountsPath())
	if err != nil {
		return err
	}

	type legacyAccountModels struct {
		Models       map[string]ModelDetails `yaml:"models"`
		CurrentModel string                  `yaml:"current-model,omitempty"`
	}
	type legacyControllerAccountModels struct {
		AccountModels map[string]*legacyAccountModels `yaml:"accounts"`
	}
	type legacyModelsCollection struct {
		ControllerAccountModels map[string]legacyControllerAccountModels `yaml:"controllers"`
	}

	var legacy legacyModelsCollection
	if err := yaml.Unmarshal(data, &legacy); err != nil {
		return errors.Annotate(err, "cannot unmarshal models")
	}
	result := make(map[string]*ControllerModels)
	for controller, controllerAccountModels := range legacy.ControllerAccountModels {
		accountDetails, ok := accounts[controller]
		if !ok {
			continue
		}
		accountModels, ok := controllerAccountModels.AccountModels[accountDetails.User]
		if !ok {
			continue
		}
		result[controller] = &ControllerModels{
			accountModels.Models,
			accountModels.CurrentModel,
		}
	}
	if len(result) > 0 {
		// Only write if we found at least one,
		// which means the file was in legacy
		// format. Otherwise leave it alone.
		return WriteModelsFile(result)
	}
	return nil
}
