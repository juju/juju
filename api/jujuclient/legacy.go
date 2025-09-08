// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient

import (
	"strings"

	"github.com/juju/names/v6"

	"github.com/juju/juju/core/model"
)

// fixLegacyModelNames transforms any model names using a username as
// a model prefix to a name with a valid model qualifier.
func fixLegacyModelNames(ctrlModelsByName map[string]*ControllerModels) {
	for ctrlName, ctrlModels := range ctrlModelsByName {
		for modelName, m := range ctrlModels.Models {
			parts := strings.Split(modelName, "/")
			if len(parts) != 2 {
				continue
			}
			qualifierPart := parts[0]
			namePart := parts[1]
			fixedQualifier := model.QualifierFromUserTag(names.NewUserTag(qualifierPart)).String()
			if fixedQualifier == qualifierPart {
				continue
			}
			fixedName := QualifyModelName(fixedQualifier, namePart)
			if fixedName == modelName {
				continue
			}
			if ctrlModels.CurrentModel == modelName {
				ctrlModels.CurrentModel = fixedName
			}
			if ctrlModels.PreviousModel == modelName {
				ctrlModels.PreviousModel = fixedName
			}
			delete(ctrlModels.Models, modelName)
			ctrlModels.Models[fixedName] = m
		}
		ctrlModelsByName[ctrlName] = ctrlModels
	}
}
