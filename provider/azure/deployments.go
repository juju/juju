// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2018-05-01/resources"
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
		Properties: &resources.DeploymentProperties{
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
	if err != nil {
		return errors.Annotatef(err, "creating deployment %q", deploymentName)
	}
	return nil
}
