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

// Client provides access to the UpgradeSeries API facade.
type Client struct {
	*common.UpgradeSeriesAPI

	facade base.FacadeCaller
	// authTag contains the authenticated unit/machine tag.
	authTag names.Tag
}

// NewClient Constructs an API caller.
func NewClient(caller base.APICaller, authTag names.Tag) *Client {
	facadeCaller := base.NewFacadeCaller(
		caller,
		upgradeSeriesFacade,
	)
	return &Client{
		facade:           facadeCaller,
		authTag:          authTag,
		UpgradeSeriesAPI: common.NewUpgradeSeriesAPI(facadeCaller, authTag),
	}
}

// Machine status retrieves the machine status from remote state.
func (s *Client) MachineStatus() (model.UpgradeSeriesStatus, error) {
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
	if r.Error == nil {
		return r.Status.Status, nil
	}

	if params.IsCodeNotFound(r.Error) {
		return "", errors.NewNotFound(r.Error, "")
	}

	return "", errors.Trace(r.Error)
}

// SetMachineStatus sets the machine status in remote state.
func (s *Client) SetMachineStatus(status model.UpgradeSeriesStatus) error {
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

// StartUnitCompletion starts the complete phase for all subordinate units.
func (s *Client) StartUnitCompletion() error {
	var results params.ErrorResults
	args := params.UpgradeSeriesStatusParams{
		Params: []params.UpgradeSeriesStatus{{
			Entity: params.Entity{Tag: s.authTag.String()},
		}},
	}
	err := s.facade.FacadeCall("StartUnitCompletion", args, &results)
	if err != nil {
		return err
	}
	if len(results.Results) != 1 {
		return errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	return results.Results[0].Error
}
