// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider/scale"
)

// Scale scales the Application's unit to the value specificied. Scale must
// be >= 0. Application units will be removed or added to meet the scale
// defined.
func (a *app) Scale(scaleTo int) error {
	switch a.deploymentType {
	case caas.DeploymentStateful:
		return scale.PatchReplicasToScale(
			context.Background(),
			a.name,
			int32(scaleTo),
			scale.StatefulSetScalePatcher(a.client.AppsV1().StatefulSets(a.namespace)),
		)
	case caas.DeploymentStateless:
		return scale.PatchReplicasToScale(
			context.Background(),
			a.name,
			int32(scaleTo),
			scale.DeploymentScalePatcher(a.client.AppsV1().Deployments(a.namespace)),
		)
	default:
		return errors.NotSupportedf(
			"application %q deployment type %q cannot be scaled",
			a.name, a.deploymentType)
	}
}
