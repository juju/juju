// Copyright 2023 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	jujuerrors "github.com/juju/errors"

	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/internal/errors"
)

const controllerCharmURL = "juju-controller"

func populateControllerCharm(ctx context.Context, deployer ControllerCharmDeployer) (string, DeployCharmInfo, error) {
	controllerAddress, err := deployer.ControllerAddress(ctx)
	if err != nil {
		return "", DeployCharmInfo{}, errors.Errorf("getting controller address: %w", err)
	}

	arch := deployer.ControllerCharmArch()
	base, err := deployer.ControllerCharmBase()
	if err != nil {
		return "", DeployCharmInfo{}, errors.Errorf("getting controller charm base: %w", err)
	}

	// When deploying a local charm, it is expected that the charm is located
	// in a certain location. If the charm is not located there, we'll get an
	// error indicating that the charm is not found.
	deployInfo, err := deployer.DeployLocalCharm(ctx, arch, base)
	if err != nil && !errors.Is(err, jujuerrors.NotFound) {
		return "", DeployCharmInfo{}, errors.Errorf("deploying local controller charm: %w", err)
	}

	// If the errors is not found locally, we'll try to download it from
	// charm hub.
	if errors.Is(err, jujuerrors.NotFound) {
		deployInfo, err = deployer.DeployCharmhubCharm(ctx, arch, base)
		if err != nil {
			return "", DeployCharmInfo{}, errors.Errorf("deploying charmhub controller charm: %w", err)
		}
	}

	return controllerAddress, deployInfo, nil
}

// PopulateIAASControllerCharm is the function that is used to populate the
// controller charm.
// When deploying a local charm, it is expected that the charm is located
// in a certain location. If the charm is not located there, we'll get an
// error indicating that the charm is not found.
// If the errors is not found locally, we'll try to download it from
// charm hub.
// Once the charm is added, set up the controller application.
func PopulateIAASControllerCharm(ctx context.Context, deployer ControllerCharmDeployer) error {
	controllerAddress, deployInfo, err := populateControllerCharm(ctx, deployer)
	if err != nil {
		return errors.Errorf("populating controller charm: %w", err)
	}

	// Once the charm is added, set up the controller application.
	err = deployer.AddIAASControllerApplication(ctx, deployInfo, controllerAddress)
	if err != nil && !errors.Is(err, applicationerrors.ApplicationAlreadyExists) {
		return errors.Errorf("adding controller application: %w", err)
	}

	return nil
}

// PopulateCAASControllerCharm is the function that is used to populate the
// controller charm.
// When deploying a local charm, it is expected that the charm is located
// in a certain location. If the charm is not located there, we'll get an
// error indicating that the charm is not found.
// If the errors is not found locally, we'll try to download it from
// charm hub.
// Once the charm is added, set up the controller application.
func PopulateCAASControllerCharm(ctx context.Context, deployer ControllerCharmDeployer) error {
	controllerAddress, deployInfo, err := populateControllerCharm(ctx, deployer)
	if err != nil {
		return errors.Errorf("populating controller charm: %w", err)
	}

	// Once the charm is added, set up the controller application.
	err = deployer.AddCAASControllerApplication(ctx, deployInfo, controllerAddress)
	if err != nil && !errors.Is(err, applicationerrors.ApplicationAlreadyExists) {
		return errors.Errorf("adding controller application: %w", err)
	}

	// Finally, complete the CAAS process.
	if err := deployer.CompleteCAASProcess(ctx); err != nil {
		return errors.Errorf("completing process: %w", err)
	}

	return nil
}
