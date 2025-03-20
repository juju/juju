// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"fmt"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
)

// The controllerFacadeNames are the root names that can be accessed
// using a controller-only login. Any facade added here needs to work
// independently of individual models.
var controllerFacadeNames = set.NewStrings(
	"AllModelWatcher",
	"ApplicationOffers",
	"Cloud",
	"Controller",
	"CrossController",
	"MigrationTarget",
	"ModelManager",
	"ModelUpgrader",
	"ModelSummaryWatcher",
	"SecretBackends",
	"UserManager",
)

// commonFacadeNames holds root names that can be accessed using both
// controller and model connections.
var commonFacadeNames = set.NewStrings(
	"Pinger",
	"Bundle",

	// TODO(mjs) - bug 1632172 - Exposed for model logins for
	// backwards compatibility. Remove once we're sure no non-Juju
	// clients care about it.
	"HighAvailability",

	// NotifyWatcher may be used for watching controller API info,
	// in conjunction with the CrossController facade.
	"NotifyWatcher",

	// ModelConfig may be used for letting controller commands access provider, for example, juju add-k8s.
	"ModelConfig",

	"SSHServer",
)

func controllerFacadesOnly(facadeName, _ string) error {
	if !IsControllerFacade(facadeName) {
		return errors.NewNotSupported(nil, fmt.Sprintf("facade %q not supported for controller API connection", facadeName))
	}
	return nil
}

// IsControllerFacade reports whether the given facade name can be accessed
// using a controller connection.
func IsControllerFacade(facadeName string) bool {
	return controllerFacadeNames.Contains(facadeName) || commonFacadeNames.Contains(facadeName)
}
