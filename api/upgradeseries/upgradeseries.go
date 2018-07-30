// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package upgradeseries

import (
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
)

const upgradeSeriesFacade = "UpgradeSeries"

// State provides access to the UpgradeSeries API facade.
type State struct {
	*common.UpgradeSeriesAPI

	facade base.FacadeCaller
	// unitTag contains the authenticated unit's tag.
	authTag names.Tag
}

func NewState(
	caller base.APICaller,
	authTag names.Tag,
) *State {
	facadeCaller := base.NewFacadeCaller(
		caller,
		upgradeSeriesFacade,
	)
	return &State{
		facade:           facadeCaller,
		authTag:          authTag,
		UpgradeSeriesAPI: common.NewUpgradeSeriesAPI(facadeCaller, authTag),
	}
}
