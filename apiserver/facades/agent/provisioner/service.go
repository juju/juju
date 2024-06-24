// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"context"

	"github.com/juju/juju/core/containermanager"
	"github.com/juju/juju/core/instance"
)

type AgentProvisionerService interface {
	ContainerManagerConfigForType(context.Context, instance.ContainerType) (containermanager.Config, error)
}
