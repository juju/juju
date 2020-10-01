// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"github.com/juju/romulus"

	"github.com/juju/juju/api/application"
	apicharms "github.com/juju/juju/api/charms"
)

func Steps() []DeployStep {
	return []DeployStep{
		&RegisterMeteredCharm{
			PlanURL:      romulus.DefaultAPIRoot,
			RegisterPath: "/plan/authorize",
			QueryPath:    "/charm",
		},
		&ValidateLXDProfileCharm{},
	}
}

// DeploymentInfo is used to maintain all deployment information for
// deployment steps.
type DeploymentInfo struct {
	CharmID         application.CharmID
	ApplicationName string
	ModelUUID       string
	CharmInfo       *apicharms.CharmInfo
	ApplicationPlan string
	Force           bool
}
