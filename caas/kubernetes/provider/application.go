// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/version/v2"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider/application"
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

func (k *kubernetesClient) upgradeApplication(agentTag names.Tag, vers version.Number) error {
	appName, err := names.UnitApplication(agentTag.Id())
	if err != nil {
		return errors.Trace(err)
	}
	app := k.Application(
		appName,
		caas.DeploymentStateful, // TODO(embedded): we hardcode it to stateful for now, it needs to be fixed soon!
	)
	return app.Upgrade(vers)
}
