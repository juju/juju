// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"github.com/juju/juju/api/client/application"
	apicommoncharms "github.com/juju/juju/api/common/charms"
)

// DeploymentInfo is used to maintain all deployment information for
// deployment steps.
type DeploymentInfo struct {
	CharmID         application.CharmID
	ApplicationName string
	ModelUUID       string
	CharmInfo       *apicommoncharms.CharmInfo
	ApplicationPlan string
	Force           bool
}
