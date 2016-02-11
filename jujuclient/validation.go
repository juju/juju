// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient

import (
	"github.com/juju/errors"
)

// ValidateControllerDetails ensures that given controller details are valid.
func ValidateControllerDetails(details ControllerDetails) error {
	if details.ControllerUUID == "" {
		return errors.NotValidf("missing uuid, controller details")
	}
	if details.CACert == "" {
		return errors.NotValidf("missing ca-cert, controller details")
	}
	return nil
}

// ValidateModelDetails ensures that given controller details are valid.
func ValidateModelDetails(details ModelDetails) error {
	if details.ModelUUID == "" {
		return errors.NotValidf("missing uuid, model details")
	}
	return nil
}

// ValidateControllerName validates the given controller name.
func ValidateControllerName(name string) error {
	// TODO(axw) define a regex for valid controller names.
	if name == "" {
		return errors.NotValidf("empty controller name")
	}
	return nil
}

// ValidateModelName validates the given model name.
func ValidateModelName(name string) error {
	// TODO(axw) define a regex for valid model names.
	if name == "" {
		return errors.NotValidf("empty model name")
	}
	return nil
}
