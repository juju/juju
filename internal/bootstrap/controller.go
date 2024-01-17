// Copyright 2023 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/juju/errors"
)

const controllerCharmURL = "juju-controller"

// PopulateControllerCharm is the function that is used to populate the
// controller charm.
// When deploying a local charm, it is expected that the charm is located
// in a certain location. If the charm is not located there, we'll get an
// error indicating that the charm is not found.
// If the errors is not found locally, we'll try to download it from
// charm hub.
// Once the charm is added, set up the controller application.
func PopulateControllerCharm(ctx context.Context, deployer ControllerCharmDeployer) error {
	controllerAddress, err := deployer.ControllerAddress(ctx)
	if err != nil {
		return errors.Annotatef(err, "getting controller address")
	}

	arch := deployer.ControllerCharmArch()
	base, err := deployer.ControllerCharmBase()
	if err != nil {
		return errors.Annotatef(err, "getting controller charm base")
	}

	// When deploying a local charm, it is expected that the charm is located
	// in a certain location. If the charm is not located there, we'll get an
	// error indicating that the charm is not found.
	curl, origin, err := deployer.DeployLocalCharm(ctx, arch, base)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return errors.Annotatef(err, "deploying local controller charm")
	}
	// If the errors is not found locally, we'll try to download it from
	// charm hub.
	if errors.Is(err, errors.NotFound) {
		curl, origin, err = deployer.DeployCharmhubCharm(ctx, arch, base)
		if err != nil {
			return errors.Annotatef(err, "deploying charmhub controller charm")
		}
	}
	if origin == nil {
		return errors.NotValidf("origin")
	}

	// Once the charm is added, set up the controller application.
	controllerUnit, err := deployer.AddControllerApplication(ctx, curl, *origin, controllerAddress)
	if err != nil {
		return errors.Annotatef(err, "adding controller application")
	}

	// Finally, complete the process.
	if err := deployer.CompleteProcess(ctx, controllerUnit); err != nil {
		return errors.Annotatef(err, "completing process")
	}
	return nil
}
