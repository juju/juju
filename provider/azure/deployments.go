// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/juju/errors"

	"github.com/juju/juju/v3/environs/context"
	"github.com/juju/juju/v3/provider/azure/internal/armtemplates"
	"github.com/juju/juju/v3/provider/azure/internal/errorutils"
)

func (env *azureEnviron) createDeployment(
	ctx context.ProviderCallContext,
	resourceGroup string,
	deploymentName string,
	t armtemplates.Template,
) error {
	deploy, err := env.deployClient()
	if err != nil {
		return errors.Trace(err)
	}
	templateMap, err := t.Map()
	if err != nil {
		return errors.Trace(err)
	}
	deployment := armresources.Deployment{
		Properties: &armresources.DeploymentProperties{
			Template: &templateMap,
			Mode:     to.Ptr(armresources.DeploymentModeIncremental),
		},
	}
	poller, err := deploy.BeginCreateOrUpdate(
		ctx,
		resourceGroup,
		deploymentName,
		deployment,
		nil,
	)
	// We only want to wait for deployments which are not shared
	// resources, otherwise add model operations will be held up.
	if err == nil && deploymentName != commonDeployment {
		_, err = poller.PollUntilDone(ctx, nil)
	}
	return errorutils.HandleCredentialError(errors.Annotatef(err, "creating Azure deployment %q", deploymentName), ctx)
}
