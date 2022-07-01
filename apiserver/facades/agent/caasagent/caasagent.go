// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasagent

import (
	"github.com/juju/juju/v2/apiserver/common"
	"github.com/juju/juju/v2/apiserver/common/cloudspec"
	"github.com/juju/juju/v2/apiserver/facade"
)

// FacadeV2 is the V2 facade of the caas agent
type FacadeV2 struct {
	auth      facade.Authorizer
	resources facade.Resources
	cloudspec.CloudSpecer
	*common.ModelWatcher
	*common.ControllerConfigAPI
}

// FacadeV1 is the V1 facade of the caas agent
type FacadeV1 struct {
	*FacadeV2
}
