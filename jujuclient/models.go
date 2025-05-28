// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient

import (
	"fmt"
	"os"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/utils/v4"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/core/model"
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
	data, err := os.ReadFile(file)
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
	if err := migrateLocalModelUsers(models); err != nil {
		return nil, err
	}
	if err := addModelType(models); err != nil {
		return nil, err
	}
	return models, nil
}

// addModelType adds missing model type data if necessary.
func addModelType(models map[string]*ControllerModels) error {
	changes := false
	for _, cm := range models {
		for name, m := range cm.Models {
			if m.ModelType == "" {
				changes = true
				m.ModelType = model.IAAS
				cm.Models[name] = m
			}
		}
	}
	if changes {
		return WriteModelsFile(models)
	}
	return nil
}

// migrateLocalModelUsers strips any @local domains from any qualified model names.
func migrateLocalModelUsers(usermodels map[string]*ControllerModels) error {
	changes := false
	for _, userModel := range usermodels {
		for name, modelDetails := range userModel.Models {
			migratedName, changed, err := migrateModelName(name)
			if err != nil {
				return errors.Trace(err)
			}
			if !changed {
				continue
			}
			delete(userModel.Models, name)
			userModel.Models[migratedName] = modelDetails
			changes = true
		}
		migratedName, changed, err := migrateModelName(userModel.CurrentModel)
		if err != nil {
			return errors.Trace(err)
		}
		if !changed {
			continue
		}
		userModel.CurrentModel = migratedName
	}
	if changes {
		return WriteModelsFile(usermodels)
	}
	return nil
}

func migrateModelName(legacyName string) (string, bool, error) {
	i := strings.IndexRune(legacyName, '/')
	if i < 0 {
		return legacyName, false, nil
	}
	owner := legacyName[:i]
	if !names.IsValidUser(owner) {
		return "", false, errors.NotValidf("user name %q", owner)
	}
	if !strings.HasSuffix(owner, "@local") {
		return legacyName, false, nil
	}
	rawModelName := legacyName[i+1:]
	return QualifyModelName(names.NewUserTag(owner).Id(), rawModelName), true, nil
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

	// PreviousModel is the name of the previous model for the account.
	PreviousModel string `yaml:"previous-model,omitempty"`
}

// QualifyModelName returns a model name qualified with the model qualifier.
func QualifyModelName(qualifier string, modelName string) string {
	return fmt.Sprintf("%s/%s", qualifier, modelName)
}

// IsQualifiedModelName returns true if the provided model name is qualified
// with an owner. The name is assumed to be either a valid qualified model
// name, or a valid unqualified model name.
func IsQualifiedModelName(name string) bool {
	return strings.ContainsRune(name, '/')
}

// SplitModelName splits a qualified model name into the model and namespace
// name components.
func SplitModelName(name string) (string, string, error) {
	i := strings.IndexRune(name, '/')
	if i < 0 {
		return "", "", errors.NotValidf("unqualified model name %q", name)
	}
	qualifier := name[:i]
	if !model.IsValidQualifier(qualifier) {
		return "", "", errors.NotValidf("qualifier %q", qualifier)
	}
	name = name[i+1:]
	return name, qualifier, nil
}
