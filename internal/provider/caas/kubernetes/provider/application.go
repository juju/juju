// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"github.com/juju/juju/internal/provider/caas"
	"github.com/juju/juju/internal/provider/caas/kubernetes/provider/application"
)

// Application returns an Application interface.
func (k *kubernetesClient) Application(name string, deploymentType caas.DeploymentType) caas.Application {
	return application.NewApplication(name,
		k.namespace,
		k.modelUUID,
		k.CurrentModel(),
		k.IsLegacyLabels(),
		deploymentType,
		k.client(),
		k.newWatcher,
		k.clock,
		k.randomPrefix,
	)
}
