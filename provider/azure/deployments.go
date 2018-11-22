// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2017-05-10/resources"
	"github.com/juju/errors"

	"github.com/juju/juju/provider/azure/internal/armtemplates"
)

func createDeployment(
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
		&resources.DeploymentProperties{
			Template: &templateMap,
			Mode:     resources.Incremental,
		},
	}
	ctx := context.Background()
	_, err = client.CreateOrUpdate(
		ctx,
		resourceGroup,
		deploymentName,
		deployment,
	)
	// veebers: I think its fine that we're ignoring the result future.
	if err != nil {
		return errors.Annotatef(err, "creating deployment %q", deploymentName)
	}
	return nil
}
