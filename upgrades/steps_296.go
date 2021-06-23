// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	ctx "context"

	"github.com/juju/errors"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/environs"
)

// upgradeKubernetesClusterCredential is an upgrade step interface that
// Kubernetes providers need to implement to perform in cluster credential
// upgrades
type upgradeKubernetesClusterCredential interface {
	InClusterCredentialUpgrade() error
}

// stateStepsFor296 returns upgrade steps for juju 2.9.6
func stateStepsFor296() []Step {
	return []Step{
		&upgradeStep{
			description: "prepare k8s controller for in cluster credentials",
			targets:     []Target{DatabaseMaster},
			run:         controllerInClusterCredentials,
		},
	}
}

// controllerInClusterCredentials performs the upgrade for Kubernetes
// controllers to us in cluster credentials
func controllerInClusterCredentials(context Context) error {
	cloudSpec, modelCfg, uuid, err := context.State().KubernetesInClusterCredentialSpec()
	if errors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}

	cloudSpec.IsControllerCloud = false
	broker, err := caas.New(ctx.TODO(), environs.OpenParams{
		ControllerUUID: uuid,
		Cloud:          cloudSpec,
		Config:         modelCfg,
	})
	if err != nil {
		return errors.Trace(err)
	}

	upgrader, ok := broker.(upgradeKubernetesClusterCredential)
	if !ok {
		return errors.New("caas broker does not implement kubernetes cluster credential upgrader")
	}

	return upgrader.InClusterCredentialUpgrade()
}
