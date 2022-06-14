// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2020-10-01/resources"
	"github.com/juju/errors"

	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/provider/azure/internal/armtemplates"
	"github.com/juju/juju/provider/azure/internal/errorutils"
)

func createDeployment(
	ctx context.ProviderCallContext,
	client resources.DeploymentsClient,
	resourceGroup string,
	deploymentName string,
	t armtemplates.Template,
) error {
	templateMap, err := t.Map()
	if err != nil {
		return errors.Trace(err)
	}
	deployment := resources.Deployment{
		Properties: &resources.DeploymentProperties{
			Template: &templateMap,
			Mode:     resources.DeploymentModeIncremental,
		},
	}
	deployFuture, err := client.CreateOrUpdate(
		ctx,
		resourceGroup,
		deploymentName,
		deployment,
	)
	if err != nil {
		return errorutils.HandleCredentialError(errors.Annotatef(err, "creating Azure deployment %q", deploymentName), ctx)
	}
	err = deployFuture.WaitForCompletionRef(ctx, client.Client)
	return errors.Annotatef(err, "creating Azure deployment %q", deploymentName)
}
