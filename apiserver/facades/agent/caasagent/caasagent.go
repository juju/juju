// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasagent

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/cloudspec"
	"github.com/juju/juju/apiserver/common/model"
)

// FacadeV2 is the V2 facade of the caas agent
type FacadeV2 struct {
	cloudspec.CloudSpecer
	*model.ModelConfigWatcher
	*common.ControllerConfigAPI
}
