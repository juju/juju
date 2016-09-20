// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
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
	return nil
}

// ValidateModelDetails ensures that given account details are valid.
func ValidateAccountDetails(details AccountDetails) error {
	if err := validateUser(details.User); err != nil {
		return errors.Trace(err)
	}
	// It is valid for a password to be blank, because the client
	// may use macaroons instead.
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
	modelName, owner, err := SplitModelName(name)
	if err != nil {
		return errors.Annotatef(err, "validating model name %q", name)
	}
	if err := validateUserTag(owner); err != nil {
		err = errors.Annotate(err, "validating model owner name")
		return errors.Annotatef(err, "validating model name %q", name)
	}
	if !names.IsValidModelName(modelName) {
		return errors.NotValidf("model name %q", name)
	}
	return nil
}

// ValidateAccountName validates the given account name.
func ValidateAccountName(name string) error {
	// An account name is a domain-qualified user, e.g. bob@local.
	return validateUser(name)
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
	tag := names.NewUserTag(name)
	return validateUserTag(tag)
}

func validateUserTag(tag names.UserTag) error {
	if tag.Id() != tag.Canonical() {
		return errors.NotValidf("unqualified user name %q", tag.Id())
	}
	return nil
}
