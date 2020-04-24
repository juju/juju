// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
)

// UpgradeSeriesAPI provides common agent-side API functions to
// call into apiserver.common/UpgradeSeries
type UpgradeSeriesAPI struct {
	facade base.FacadeCaller
	tag    names.Tag
}

// NewUpgradeSeriesAPI creates a UpgradeSeriesAPI on the specified facade,
// and uses this name when calling through the caller.
func NewUpgradeSeriesAPI(facade base.FacadeCaller, tag names.Tag) *UpgradeSeriesAPI {
	return &UpgradeSeriesAPI{facade: facade, tag: tag}
}

// WatchUpgradeSeriesNotifications returns a NotifyWatcher for observing the state of
// a series upgrade.
func (u *UpgradeSeriesAPI) WatchUpgradeSeriesNotifications() (watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.facade.FacadeCall("WatchUpgradeSeriesNotifications", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewNotifyWatcher(u.facade.RawAPICaller(), result)
	return w, nil
}

// UpgradeSeriesUnitStatus returns the upgrade series status of a
// unit from remote state.
func (u *UpgradeSeriesAPI) UpgradeSeriesUnitStatus() ([]model.UpgradeSeriesStatus, error) {
	var results params.UpgradeSeriesStatusResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}

	err := u.facade.FacadeCall("UpgradeSeriesUnitStatus", args, &results)
	if err != nil {
		return nil, err
	}

	statuses := make([]model.UpgradeSeriesStatus, len(results.Results))
	for i, res := range results.Results {
		if res.Error != nil {
			// TODO (externalreality) The code to convert api errors (with
			// error codes) back to normal Go errors is in bad spot and
			// causes import cycles which is why we don't use it here and may
			// be the reason why it has few uses despite being useful.
			if params.IsCodeNotFound(res.Error) {
				return nil, errors.NewNotFound(res.Error, "")
			}
			return nil, res.Error
		}
		statuses[i] = res.Status
	}
	return statuses, nil
}

// SetUpgradeSeriesUnitStatus sets the upgrade series status of the
// unit in the remote state.
func (u *UpgradeSeriesAPI) SetUpgradeSeriesUnitStatus(status model.UpgradeSeriesStatus, reason string) error {
	var results params.ErrorResults
	args := params.UpgradeSeriesStatusParams{
		Params: []params.UpgradeSeriesStatusParam{{
			Entity:  params.Entity{Tag: u.tag.String()},
			Status:  status,
			Message: reason,
		}},
	}
	err := u.facade.FacadeCall("SetUpgradeSeriesUnitStatus", args, &results)
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
