// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/juju/errors"

	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/provider/azure/internal/armtemplates"
	"github.com/juju/juju/provider/azure/internal/errorutils"
)

func createDeployment(
	ctx context.ProviderCallContext,
	client *armresources.DeploymentsClient,
	resourceGroup string,
	deploymentName string,
	t armtemplates.Template,
) error {
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
	poller, err := client.BeginCreateOrUpdate(
		ctx,
		resourceGroup,
		deploymentName,
		deployment,
		nil,
	)
	if err == nil {
		_, err = poller.PollUntilDone(ctx, nil)
	}
	return errorutils.HandleCredentialError(errors.Annotatef(err, "creating Azure deployment %q", deploymentName), ctx)
}
