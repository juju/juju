// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/jujuclient"
)

// QualifyingClientStore wraps a jujuclient.ClientStore, modifying
// model-related methods such that they accept unqualified model
// names, and automatically qualify them with the logged-in user
// name as necessary.
type QualifyingClientStore struct {
	jujuclient.ClientStore
}

// QualifiedModelName returns a Qualified model name, given either
// an unqualified or qualified model name. If the input is a
// fully qualified name, it is returned untouched; otherwise it is
// return qualified with the logged-in user name.
func (s QualifyingClientStore) QualifiedModelName(controllerName, modelName string) (string, error) {
	if !jujuclient.IsQualifiedModelName(modelName) {
		details, err := s.ClientStore.AccountDetails(controllerName)
		if err != nil {
			return "", errors.Annotate(err, "getting account details for qualifying model name")
		}
		owner := names.NewUserTag(details.User)
		modelName = jujuclient.JoinOwnerModelName(owner, modelName)
	} else {
		unqualifiedModelName, owner, err := jujuclient.SplitModelName(modelName)
		if err != nil {
			return "", errors.Trace(err)
		}
		owner = names.NewUserTag(owner.Id())
		modelName = jujuclient.JoinOwnerModelName(owner, unqualifiedModelName)
	}
	return modelName, nil
}

// Implements jujuclient.ModelGetter.
func (s QualifyingClientStore) ModelByName(controllerName, modelName string) (*jujuclient.ModelDetails, error) {
	qualifiedModelName, err := s.QualifiedModelName(controllerName, modelName)
	if err != nil {
		return nil, errors.Annotatef(err, "getting model %q", modelName)
	}
	return s.ClientStore.ModelByName(controllerName, qualifiedModelName)
}

// Implements jujuclient.ModelUpdater.
func (s QualifyingClientStore) UpdateModel(controllerName, modelName string, details jujuclient.ModelDetails) error {
	qualifiedModelName, err := s.QualifiedModelName(controllerName, modelName)
	if err != nil {
		return errors.Annotatef(err, "updating model %q", modelName)
	}
	return s.ClientStore.UpdateModel(controllerName, qualifiedModelName, details)
}

// Implements jujuclient.ModelUpdater.
func (s QualifyingClientStore) SetModels(controllerName string, models map[string]jujuclient.ModelDetails) error {
	qualified := make(map[string]jujuclient.ModelDetails, len(models))
	for name, details := range models {
		modelName, err := s.QualifiedModelName(controllerName, name)
		if err != nil {
			return errors.Annotatef(err, "updating model %q", name)
		}
		qualified[modelName] = details
	}
	return s.ClientStore.SetModels(controllerName, models)
}

// Implements jujuclient.ModelUpdater.
func (s QualifyingClientStore) SetCurrentModel(controllerName, modelName string) error {
	qualifiedModelName, err := s.QualifiedModelName(controllerName, modelName)
	if err != nil {
		return errors.Annotatef(err, "setting current model to %q", modelName)
	}
	return s.ClientStore.SetCurrentModel(controllerName, qualifiedModelName)
}

// Implements jujuclient.ModelRemover.
func (s QualifyingClientStore) RemoveModel(controllerName, modelName string) error {
	qualifiedModelName, err := s.QualifiedModelName(controllerName, modelName)
	if err != nil {
		return errors.Annotatef(err, "removing model %q", modelName)
	}
	return s.ClientStore.RemoveModel(controllerName, qualifiedModelName)
}
