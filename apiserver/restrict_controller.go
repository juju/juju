// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/utils/set"
)

// The controllerFacadeNames are the root names that can be accessed
// using a controller-only login. Any facade added here needs to work
// independently of individual models.
var controllerFacadeNames = set.NewStrings(
	"AllModelWatcher",
	"Cloud",
	"Controller",
	"MigrationTarget",
	"ModelManager",
	"UserManager",
)

func controllerFacadesOnly(facadeName, _ string) error {
	if !isControllerFacade(facadeName) {
		return errors.NewNotSupported(nil, fmt.Sprintf("facade %q not supported for controller API connection", facadeName))
	}
	return nil
}

func isControllerFacade(facadeName string) bool {
	// Note: the Pinger facade can be used in both model and controller
	// connections.
	return controllerFacadeNames.Contains(facadeName) || facadeName == "Pinger"
}
