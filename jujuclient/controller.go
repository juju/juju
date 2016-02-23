// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient

import (
	"github.com/juju/errors"
)

// LocalControllerByName returns local controller given a name that may omit the "local." prefix.
func LocalControllerByName(store ControllerStore, controllerName string) (string, *ControllerDetails, error) {
	result, err := store.ControllerByName(controllerName)
	if err == nil {
		return controllerName, result, nil
	}
	if !errors.IsNotFound(err) {
		return "", nil, err
	}
	var secondErr error
	localName := "local." + controllerName
	result, secondErr = store.ControllerByName(localName)
	// If fallback name not found, return the original error.
	if errors.IsNotFound(secondErr) {
		return "", nil, err
	}
	return localName, result, secondErr
}
