// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient

import (
	"github.com/juju/errors"
	"github.com/juju/names/v6"
)

// ValidateControllerDetails ensures that given controller details are valid.
func ValidateControllerDetails(details ControllerDetails) error {
	if details.ControllerUUID == "" {
		return errors.NotValidf("missing uuid, controller details")
	}
	return nil
}

// ValidateModelDetails ensures that given model details are valid.
func ValidateModelDetails(details ModelDetails) error {
	if details.ModelUUID == "" {
		return errors.NotValidf("missing uuid, model details")
	}
	if details.ModelType == "" {
		return errors.NotValidf("missing type, model details")
	}
	return nil
}

// ValidateModelDetails ensures that given account details are valid.
func ValidateAccountDetails(details AccountDetails) error {
	// If a session token is provided, it is valid for the user field to
	// be empty. Only validate the user when no session token is provided.
	if details.SessionToken == "" {
		if err := validateUser(details.User); err != nil {
			return errors.Trace(err)
		}
	}
	// It is valid for a password to be blank, because the client
	// may use macaroons or OIDC login instead.
	return nil
}

// ValidateControllerName validates the given controller name.
func ValidateControllerName(name string) error {
	// Note: the only validation we can do here is to check if no name was supplied.
	// We can accept any names here, irrespective of names.IsValidControllerName
	// since controller names can also be DNS names containing "." or
	// any other character combinations.
	if name == "" {
		return errors.NotValidf("empty controller name")
	}
	return nil
}

// ValidateModelName validates the given model name.
func ValidateModelName(name string) error {
	modelName, _, err := SplitFullyQualifiedModelName(name)
	if err != nil {
		return errors.Annotatef(err, "validating model name %q", name)
	}
	if !names.IsValidModelName(modelName) {
		return errors.NotValidf("model name %q", name)
	}
	return nil
}

// ValidateModel validates the given model name and details.
func ValidateModel(name string, details ModelDetails) error {
	if err := ValidateModelName(name); err != nil {
		return err
	}
	if err := ValidateModelDetails(details); err != nil {
		return err
	}
	return nil
}

// ValidateBootstrapConfig validates the given boostrap config.
func ValidateBootstrapConfig(cfg BootstrapConfig) error {
	if cfg.Cloud == "" {
		return errors.NotValidf("empty cloud name")
	}
	if cfg.CloudType == "" {
		return errors.NotValidf("empty cloud type")
	}
	if len(cfg.Config) == 0 {
		return errors.NotValidf("empty config")
	}
	return nil
}

func validateUser(name string) error {
	if !names.IsValidUser(name) {
		return errors.NotValidf("user name %q", name)
	}
	return nil
}
