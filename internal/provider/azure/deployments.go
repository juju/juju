// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/juju/errors"

	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/internal/provider/azure/internal/armtemplates"
)

func (env *azureEnviron) createDeployment(
	ctx context.Context,
	resourceGroup string,
	deploymentName string,
	t armtemplates.Template,
) error {
	deploy, err := env.deployClient()
	if err != nil {
		return errors.Trace(err)
	}
	t.Schema = armtemplates.Schema
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
	var result armresources.DeploymentsClientCreateOrUpdateResponse
	if err == nil && deploymentName != commonDeployment {
		result, err = poller.PollUntilDone(ctx, nil)
		if err == nil && result.Properties != nil && result.Properties.Error != nil {
			err = errors.New(toValue(result.Properties.Error.Message))
		}
	}
	return env.HandleCredentialError(ctx, errors.Annotatef(err, "creating Azure deployment %q", deploymentName))
}

func (env *azureEnviron) createSubscriptionDeployment(
	ctx envcontext.ProviderCallContext,
	location string,
	deploymentName string,
	params any,
	t armtemplates.Template,
) error {
	deploy, err := env.deployClient()
	if err != nil {
		return errors.Trace(err)
	}
	t.Schema = armtemplates.SubscriptionSchema
	templateMap, err := t.Map()
	if err != nil {
		return errors.Trace(err)
	}
	deployment := armresources.Deployment{
		Location: to.Ptr(location),
		Properties: &armresources.DeploymentProperties{
			Parameters: params,
			Template:   &templateMap,
			Mode:       to.Ptr(armresources.DeploymentModeIncremental),
		},
	}
	poller, err := deploy.BeginCreateOrUpdateAtSubscriptionScope(
		ctx,
		deploymentName,
		deployment,
		nil,
	)
	var result armresources.DeploymentsClientCreateOrUpdateAtSubscriptionScopeResponse
	if err == nil {
		result, err = poller.PollUntilDone(ctx, nil)
		if err == nil && result.Properties != nil && result.Properties.Error != nil {
			err = errors.New(toValue(result.Properties.Error.Message))
		}
	}
	return env.HandleCredentialError(ctx, errors.Annotatef(err, "creating Azure subscription deployment %q", deploymentName))
}
