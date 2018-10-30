// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
)

// LXDProfileUpgradeWatcher provides common agent-side API functions to
// call into apiserver.common/LXDProfile
type LXDProfileUpgradeWatcher struct {
	facade base.FacadeCaller
	tag    names.Tag
}

// NewLXDProfileUpgradeWatcher creates a LXDProfileUpgradeWatcher on the
// specified facade, and uses this name when calling through the caller.
func NewLXDProfileUpgradeWatcher(facade base.FacadeCaller, tag names.Tag) *LXDProfileUpgradeWatcher {
	return &LXDProfileUpgradeWatcher{facade: facade, tag: tag}
}

// WatchLXDProfileUpgradeNotifications returns a NotifyWatcher for observing the
// state of a lxd profile upgrade.
func (u *LXDProfileUpgradeWatcher) WatchLXDProfileUpgradeNotifications() (watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.facade.FacadeCall("WatchLXDProfileUpgradeNotifications", args, &results)
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

// LXDProfileUpgradeUnitStatus returns the upgrade series status of a
// unit from remote state.
func (u *LXDProfileUpgradeWatcher) LXDProfileUpgradeUnitStatus() ([]model.LXDProfileUpgradeStatus, error) {
	var results params.LXDProfileUpgradeStatusResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}

	err := u.facade.FacadeCall("LXDProfileUpgradeUnitStatus", args, &results)
	if err != nil {
		return nil, err
	}

	statuses := make([]model.LXDProfileUpgradeStatus, len(results.Results))
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
