// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries

import (
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/rpc/params"
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

// MachineStatus status retrieves the machine status from remote state.
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

// UnitsPrepared returns the units running on this machine that have
// completed their upgrade-machine preparation, and are ready to be stopped and
// have their unit agent services converted for the target series.
func (s *Client) UnitsPrepared() ([]names.UnitTag, error) {
	units, err := s.unitsInState("UnitsPrepared")
	return units, errors.Trace(err)
}

// UnitsCompleted returns the units running on this machine that have completed
// the upgrade-machine workflow and are in their normal running state.
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

// SetMachineStatus sets the series upgrade status in remote state.
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
	base, err := corebase.GetBaseFromSeries(hostSeries)
	if err != nil {
		return errors.Trace(err)
	}
	args := params.UpdateChannelArgs{Args: []params.UpdateChannelArg{{
		Entity:  params.Entity{Tag: s.authTag.String()},
		Channel: base.Channel.Track,
	}}}

	err = s.facade.FacadeCall("FinishUpgradeSeries", args, &results)
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

// SetInstanceStatus sets the machine status in remote state.
func (s *Client) SetInstanceStatus(sts model.UpgradeSeriesStatus, msg string) error {
	var results params.ErrorResults
	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{{
			Tag:    s.authTag.String(),
			Status: string(status.Running),
			Info:   strings.Join([]string{"series upgrade ", string(sts), ": ", msg}, ""),
		}},
	}

	err := s.facade.FacadeCall("SetInstanceStatus", args, &results)
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
