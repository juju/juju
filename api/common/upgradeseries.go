// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/watcher"
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

// WatchActionNotifications returns a StringsWatcher for observing the state of
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

// UpgradeSeriesStatus returns the upgrade series status of a unit from remote state
func (u *UpgradeSeriesAPI) UpgradeSeriesStatus() ([]string, error) {
	var results params.UpgradeSeriesStatusResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}

	err := u.facade.FacadeCall("UpgradeSeriesPrepareStatus", args, &results)
	if err != nil {
		return nil, err
	}

	statuses := make([]string, len(results.Results))
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
	// TODO (manadart 2018-08-02) Should we be converting these back to
	// model.UnitSeriesUpgradeStatus and reporting an error if that fails?
	// (externalreality): validation does take place a few levels down.
	return statuses, nil
}

// SetUpgradeSeriesStatus sets the upgrade series status of the unit in the remote state
func (u *UpgradeSeriesAPI) SetUpgradeSeriesStatus(status string) error {
	var results params.ErrorResults
	args := params.SetUpgradeSeriesStatusParams{
		Params: []params.SetUpgradeSeriesStatusParam{{
			Entity: params.Entity{Tag: u.tag.String()},
			Status: status,
		}},
	}
	err := u.facade.FacadeCall("SetUpgradeSeriesPrepareStatus", args, &results)
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
