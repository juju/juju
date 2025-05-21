// Copyright 2023 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/environs/bootstrap"
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
func PopulateControllerCharm(ctx context.Context, deployer ControllerCharmDeployer) (network.ProviderAddresses, error) {
	controllerAddress, err := deployer.ControllerAddress(ctx)
	if err != nil {
		return nil, errors.Annotatef(err, "getting controller address")
	}

	arch := deployer.ControllerCharmArch()
	base, err := deployer.ControllerCharmBase()
	if err != nil {
		return nil, errors.Annotatef(err, "getting controller charm base")
	}

	// When deploying a local charm, it is expected that the charm is located
	// in a certain location. If the charm is not located there, we'll get an
	// error indicating that the charm is not found.
	deployInfo, err := deployer.DeployLocalCharm(ctx, arch, base)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return nil, errors.Annotatef(err, "deploying local controller charm")
	}

	// If the errors is not found locally, we'll try to download it from
	// charm hub.
	if errors.Is(err, errors.NotFound) {
		deployInfo, err = deployer.DeployCharmhubCharm(ctx, arch, base)
		if err != nil {
			return nil, errors.Annotatef(err, "deploying charmhub controller charm")
		}
	}

	// Once the charm is added, set up the controller application.
	if err := deployer.AddControllerApplication(ctx, deployInfo, controllerAddress); err != nil && !errors.Is(err, applicationerrors.ApplicationAlreadyExists) {
		return nil, errors.Annotatef(err, "adding controller application")
	}

	// We can deduce that the unit name must be controller/0 since we're
	// currently bootstrapping the controller, so this unit is the first unit
	// to be created.
	controllerUnitName, err := coreunit.NewNameFromParts(bootstrap.ControllerApplicationName, 0)
	if err != nil {
		return nil, errors.Errorf("creating unit name %q: %w", bootstrap.ControllerApplicationName, err)
	}
	// Finally, complete the process.
	addrs, err := deployer.CompleteProcess(ctx, controllerUnitName)
	if err != nil {
		return nil, errors.Annotatef(err, "completing process")
	}

	return addrs, nil
}
