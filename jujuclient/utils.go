// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient

import (
	"github.com/juju/errors"
)

func updateControllerModels(storedModels *ControllerModels, newModels map[string]ModelDetails) (bool, error) {
	if len(storedModels.Models) == 0 && len(newModels) == 0 {
		// Special case: no update necessary.
		return false, nil
	}
	// Add or update controller models based on a new collection.
	for modelName, details := range newModels {
		if err := ValidateModel(modelName, details); err != nil {
			// TODO what to do here?... don't want to stop
			// an update if one name is invalid..
			return false, errors.Trace(err)
		}
		storedModels.Models[modelName] = details
	}
	// Delete models that are not in the new collection.
	for modelName, _ := range storedModels.Models {
		if _, ok := newModels[modelName]; !ok {
			delete(storedModels.Models, modelName)
		}
	}
	return true, nil
}
