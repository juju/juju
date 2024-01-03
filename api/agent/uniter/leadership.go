// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// NewLeadershipSettings returns a new LeadershipSettings.
func NewLeadershipSettings(
	caller FacadeCallFn,
	newWatcher NewNotifyWatcherFn,
) *LeadershipSettings {
	return &LeadershipSettings{caller, newWatcher}
}

type FacadeCallFn func(ctx context.Context, request string, params, response interface{}) error
type NewNotifyWatcherFn func(params.NotifyWatchResult) watcher.NotifyWatcher

// LeadershipSettings provides a type that can make RPC calls
// to a service which can read, write, and watch leadership settings.
type LeadershipSettings struct {
	facadeCaller     FacadeCallFn
	newNotifyWatcher NewNotifyWatcherFn
}

// Merge merges the provided settings into the leadership settings for
// the given application and unit. Only leaders of a given application may perform
// this operation.
func (ls *LeadershipSettings) Merge(appId, unitId string, settings map[string]string) error {
	results, err := ls.bulkMerge(ls.prepareMerge(appId, unitId, settings))
	if err != nil {
		return errors.Annotatef(err, "failed to call leadership api")
	}
	if count := len(results.Results); count != 1 {
		return errors.Errorf("expected 1 result from leadership api, got %d", count)
	}
	if results.Results[0].Error != nil {
		return errors.Annotatef(results.Results[0].Error, "failed to merge leadership settings")
	}
	return nil
}

// Read retrieves the leadership settings for the given application
// ID. Anyone may perform this operation.
func (ls *LeadershipSettings) Read(appId string) (map[string]string, error) {
	results, err := ls.bulkRead(ls.prepareRead(appId))
	if err != nil {
		return nil, errors.Annotatef(err, "failed to call leadership api")
	}
	if count := len(results.Results); count != 1 {
		return nil, errors.Errorf("expected 1 result from leadership api, got %d", count)
	}
	if results.Results[0].Error != nil {
		return nil, errors.Annotatef(results.Results[0].Error, "failed to read leadership settings")
	}
	return results.Results[0].Settings, nil
}

// WatchLeadershipSettings returns a watcher which can be used to wait
// for leadership settings changes to be made for a given application ID.
func (ls *LeadershipSettings) WatchLeadershipSettings(appId string) (watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	if err := ls.facadeCaller(
		context.TODO(),
		"WatchLeadershipSettings",
		params.Entities{[]params.Entity{{names.NewApplicationTag(appId).String()}}},
		&results,
	); err != nil {
		return nil, errors.Annotate(err, "failed to call leadership api")
	}
	if count := len(results.Results); count != 1 {
		return nil, errors.Errorf("expected 1 result from leadership api, got %d", count)
	}
	if results.Results[0].Error != nil {
		return nil, errors.Annotatef(results.Results[0].Error, "failed to watch leadership settings")
	}
	return ls.newNotifyWatcher(results.Results[0]), nil
}

//
// Prepare functions for building bulk-calls.
//

func (ls *LeadershipSettings) prepareMerge(appId, unitId string, settings map[string]string) params.MergeLeadershipSettingsParam {
	return params.MergeLeadershipSettingsParam{
		ApplicationTag: names.NewApplicationTag(appId).String(),
		UnitTag:        names.NewUnitTag(unitId).String(),
		Settings:       settings,
	}
}

func (ls *LeadershipSettings) prepareRead(appId string) params.Entity {
	return params.Entity{Tag: names.NewApplicationTag(appId).String()}
}

//
// Bulk calls.
//

func (ls *LeadershipSettings) bulkMerge(args ...params.MergeLeadershipSettingsParam) (*params.ErrorResults, error) {
	// Don't make the jump over the network if we don't have to.
	if len(args) <= 0 {
		return &params.ErrorResults{}, nil
	}

	bulkArgs := params.MergeLeadershipSettingsBulkParams{Params: args}
	var results params.ErrorResults
	return &results, ls.facadeCaller(context.TODO(), "Merge", bulkArgs, &results)
}

func (ls *LeadershipSettings) bulkRead(args ...params.Entity) (*params.GetLeadershipSettingsBulkResults, error) {

	// Don't make the jump over the network if we don't have to.
	if len(args) <= 0 {
		return &params.GetLeadershipSettingsBulkResults{}, nil
	}

	bulkArgs := params.Entities{Entities: args}
	var results params.GetLeadershipSettingsBulkResults
	return &results, ls.facadeCaller(context.TODO(), "Read", bulkArgs, &results)
}
