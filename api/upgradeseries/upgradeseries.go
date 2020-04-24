// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/model"
)

const upgradeSeriesFacade = "UpgradeSeries"

// Client provides access to the UpgradeSeries API facade.
type Client struct {
	*common.UpgradeSeriesAPI
	*common.LeadershipPinningAPI

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
		facade:               facadeCaller,
		authTag:              authTag,
		UpgradeSeriesAPI:     common.NewUpgradeSeriesAPI(facadeCaller, authTag),
		LeadershipPinningAPI: common.NewLeadershipPinningAPIFromFacade(facadeCaller),
	}
}

// Machine status retrieves the machine status from remote state.
func (s *Client) MachineStatus() (model.UpgradeSeriesStatus, error) {
	var results params.UpgradeSeriesStatusResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: s.authTag.String()}},
	}

	err := s.facade.FacadeCall("MachineStatus", args, &results)
	if err != nil {
		return "", errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return "", errors.Errorf("expected 1 result, got %d", len(results.Results))
	}

	r := results.Results[0]
	if r.Error == nil {
		return r.Status, nil
	}

	if params.IsCodeNotFound(r.Error) {
		return "", errors.NewNotFound(r.Error, "")
	}
	return "", errors.Trace(r.Error)
}

func (s *Client) TargetSeries() (string, error) {
	var results params.StringResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: s.authTag.String()}},
	}

	err := s.facade.FacadeCall("TargetSeries", args, &results)
	if err != nil {
		return "", errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return "", errors.Errorf("expected 1 result, got %d", len(results.Results))
	}

	r := results.Results[0]
	if r.Error == nil {
		return r.Result, nil
	}

	if params.IsCodeNotFound(r.Error) {
		return "", errors.NewNotFound(r.Error, "")
	}
	return "", errors.Trace(r.Error)
}

// UnitsPrepared returns the units running on this machine that have
// completed their upgrade-series preparation, and are ready to be stopped and
// have their unit agent services converted for the target series.
func (s *Client) UnitsPrepared() ([]names.UnitTag, error) {
	units, err := s.unitsInState("UnitsPrepared")
	return units, errors.Trace(err)
}

// UnitsCompleted returns the units running on this machine that have completed
// the upgrade-series workflow and are in their normal running state.
func (s *Client) UnitsCompleted() ([]names.UnitTag, error) {
	units, err := s.unitsInState("UnitsCompleted")
	return units, errors.Trace(err)
}

func (s *Client) unitsInState(facadeMethod string) ([]names.UnitTag, error) {
	var results params.EntitiesResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: s.authTag.String()}},
	}

	err := s.facade.FacadeCall(facadeMethod, args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}

	r := results.Results[0]
	if r.Error == nil {
		tags := make([]names.UnitTag, len(r.Entities))
		for i, e := range r.Entities {
			tag, err := names.ParseUnitTag(e.Tag)
			if err != nil {
				return nil, errors.Trace(err)
			}
			tags[i] = tag
		}
		return tags, nil
	}

	if params.IsCodeNotFound(r.Error) {
		return nil, errors.NewNotFound(r.Error, "")
	}
	return nil, errors.Trace(r.Error)
}

// SetMachineStatus sets the machine status in remote state.
func (s *Client) SetMachineStatus(status model.UpgradeSeriesStatus, reason string) error {
	var results params.ErrorResults
	args := params.UpgradeSeriesStatusParams{
		Params: []params.UpgradeSeriesStatusParam{{
			Entity:  params.Entity{Tag: s.authTag.String()},
			Status:  status,
			Message: reason,
		}},
	}

	err := s.facade.FacadeCall("SetMachineStatus", args, &results)
	if err != nil {
		return err
	}
	if len(results.Results) != 1 {
		return errors.Errorf("expected 1 result, got %d", len(results.Results))
	}

	result := results.Results[0]
	if result.Error != nil {
		return result.Error
	}
	return nil
}

// StartUnitCompletion starts the complete phase for all subordinate units.
func (s *Client) StartUnitCompletion(reason string) error {
	var results params.ErrorResults
	args := params.UpgradeSeriesStartUnitCompletionParam{
		Entities: []params.Entity{{Tag: s.authTag.String()}},
		Message:  reason,
	}

	err := s.facade.FacadeCall("StartUnitCompletion", args, &results)
	if err != nil {
		return err
	}
	if len(results.Results) != 1 {
		return errors.Errorf("expected 1 result, got %d", len(results.Results))
	}

	result := results.Results[0]
	if result.Error != nil {
		return result.Error
	}
	return nil
}

// FinishUpgradeSeries notifies the controller that the upgrade process is
// completely finished, passing the current host OS series.
// We use the name "Finish" to distinguish this method from the various
// "Complete" phases.
func (s *Client) FinishUpgradeSeries(hostSeries string) error {
	var results params.ErrorResults
	args := params.UpdateSeriesArgs{Args: []params.UpdateSeriesArg{{
		Entity: params.Entity{Tag: s.authTag.String()},
		Series: hostSeries,
	}}}

	err := s.facade.FacadeCall("FinishUpgradeSeries", args, &results)
	if err != nil {
		return err
	}
	if len(results.Results) != 1 {
		return errors.Errorf("expected 1 result, got %d", len(results.Results))
	}

	result := results.Results[0]
	if result.Error != nil {
		return result.Error
	}
	return nil
}
