// Copyright 2023 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	jujuerrors "github.com/juju/errors"

	"github.com/juju/juju/internal/errors"
)

const controllerCharmURL = "juju-controller"

// PopulateControllerCharm uses the local controller charm, falling back to
// Charmhub if it is absent, and ensures the controller application is set up.
func PopulateControllerCharm(ctx context.Context, deployer ControllerCharmDeployer) error {
	deployInfo, err := populateControllerCharm(ctx, deployer)
	if err != nil {
		return errors.Errorf("populating controller charm: %w", err)
	}

	if err := deployer.EnsureControllerApplication(ctx, deployInfo); err != nil {
		return errors.Errorf("ensuring controller application: %w", err)
	}

	return nil
}

func populateControllerCharm(ctx context.Context, deployer ControllerCharmDeployer) (DeployCharmInfo, error) {
	arch := deployer.ControllerCharmArch()
	base, err := deployer.ControllerCharmBase()
	if err != nil {
		return DeployCharmInfo{}, errors.Errorf("getting controller charm base: %w", err)
	}

	// When deploying a local charm, it is expected that the charm is located
	// in a certain location. If the charm is not located there, we'll get an
	// error indicating that the charm is not found.
	deployInfo, err := deployer.DeployLocalCharm(ctx, arch, base)
	if err != nil && !errors.Is(err, jujuerrors.NotFound) {
		return DeployCharmInfo{}, errors.Errorf("deploying local controller charm: %w", err)
	}

	// If the errors is not found locally, we'll try to download it from
	// charm hub.
	if errors.Is(err, jujuerrors.NotFound) {
		deployInfo, err = deployer.DeployCharmhubCharm(ctx, arch, base)
		if err != nil {
			return DeployCharmInfo{}, errors.Errorf("deploying charmhub controller charm: %w", err)
		}
	}

	return deployInfo, nil
}
