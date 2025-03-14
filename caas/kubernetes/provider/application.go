// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider/application"
	"github.com/juju/juju/core/k8s"
)

// Application returns an Application interface.
func (k *kubernetesClient) Application(name string, deploymentType k8s.K8sDeploymentType) caas.Application {
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

// DesiredReplicas returns the desired replicas for the given application.
func (k *kubernetesClient) DesiredReplicas(name string, deploymentType k8s.K8sDeploymentType) (int, error) {
	app := k.Application(name, k8s.K8sDeploymentStateful)
	state, err := app.State()
	if err != nil {
		return -1, err
	}
	return state.DesiredReplicas, nil
}
