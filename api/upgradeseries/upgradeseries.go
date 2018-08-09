// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package upgradeseries

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/model"
)

const upgradeSeriesFacade = "UpgradeSeries"

// State provides access to the UpgradeSeries API facade.
type State struct {
	*common.UpgradeSeriesAPI

	facade base.FacadeCaller
	// authTag contains the authenticated unit/machine tag.
	authTag names.Tag
}

func NewState(caller base.APICaller, authTag names.Tag) *State {
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

func (s *State) MachineStatus() (model.UpgradeSeriesStatus, error) {
	var results params.UpgradeSeriesStatusResultsNew
	args := params.Entities{
		Entities: []params.Entity{{Tag: s.authTag.String()}},
	}

	err := s.facade.FacadeCall("MachineStatus", args, &results)
	if err != nil {
		return "", err
	}
	if len(results.Results) != 1 {
		return "", errors.Errorf("expected 1 result, got %d", len(results.Results))
	}

	r := results.Results[0]
	return r.Status.Status, r.Error
}

func (s *State) SetMachineStatus(status model.UpgradeSeriesStatus) error {
	var results params.ErrorResults
	args := params.UpgradeSeriesStatusParams{
		Params: []params.UpgradeSeriesStatus{{
			Entity: params.Entity{Tag: s.authTag.String()},
			Status: status,
		}},
	}

	err := s.facade.FacadeCall("SetMachineStatus", args, &results)
	if err != nil {
		return err
	}
	if len(results.Results) != 1 {
		return errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	return results.Results[0].Error
}
