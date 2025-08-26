// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"github.com/juju/juju/caas"
	"github.com/juju/juju/internal/provider/kubernetes/application"
)

// Application returns an Application interface.
func (k *kubernetesClient) Application(name string, deploymentType caas.DeploymentType) caas.Application {
	return application.NewApplication(name,
		k.Namespace(),
		k.ModelUUID(),
		k.ModelName(),
		k.LabelVersion(),
		deploymentType,
		k.client(),
		k.newWatcher,
		k.clock,
		k.randomPrefix,
	)
}
