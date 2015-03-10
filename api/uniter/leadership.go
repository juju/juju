// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
)

// NewLeadershipSettingsAccessor returns a new LeadershipSettingsAccessor.
func NewLeadershipSettingsAccessor(
	caller FacadeCallFn,
	newWatcher NewNotifyWatcherFn,
	checkApiVersion CheckApiVersionFn,
) *LeadershipSettingsAccessor {
	return &LeadershipSettingsAccessor{caller, newWatcher, checkApiVersion}
}

type FacadeCallFn func(request string, params, response interface{}) error
type NewNotifyWatcherFn func(params.NotifyWatchResult) watcher.NotifyWatcher
type CheckApiVersionFn func(functionName string) error

// LeadershipSettingsAccessor provides a type that can make RPC calls
// to a service which can read, write, and watch leadership settings.
type LeadershipSettingsAccessor struct {
	facadeCaller     FacadeCallFn
	newNotifyWatcher NewNotifyWatcherFn
	checkApiVersion  CheckApiVersionFn
}

// Merge merges the provided settings into the leadership settings for
// the given service ID. Only leaders of a given service may perform
// this operation.
func (lsa *LeadershipSettingsAccessor) Merge(serviceId string, settings map[string]string) error {

	if err := lsa.checkApiVersion("Merge"); err != nil {
		return errors.Annotatef(err, "cannot access leadership api")
	}

	results, err := lsa.bulkMerge(lsa.prepareMerge(serviceId, settings))
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

// Read retrieves the leadership settings for the given service
// ID. Anyone may perform this operation.
func (lsa *LeadershipSettingsAccessor) Read(serviceId string) (map[string]string, error) {

	if err := lsa.checkApiVersion("Read"); err != nil {
		return nil, errors.Annotatef(err, "cannot access leadership api")
	}

	results, err := lsa.bulkRead(lsa.prepareRead(serviceId))
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
// for leadership settings changes to be made for a given service ID.
func (lsa *LeadershipSettingsAccessor) WatchLeadershipSettings(serviceId string) (watcher.NotifyWatcher, error) {

	if err := lsa.checkApiVersion("WatchLeadershipSettings"); err != nil {
		return nil, errors.Annotatef(err, "cannot access leadership api")
	}
	var results params.NotifyWatchResults
	if err := lsa.facadeCaller(
		"WatchLeadershipSettings",
		params.Entities{[]params.Entity{{names.NewServiceTag(serviceId).String()}}},
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
	return lsa.newNotifyWatcher(results.Results[0]), nil
}

//
// Prepare functions for building bulk-calls.
//

func (lsa *LeadershipSettingsAccessor) prepareMerge(serviceId string, settings map[string]string) params.MergeLeadershipSettingsParam {
	return params.MergeLeadershipSettingsParam{
		ServiceTag: names.NewServiceTag(serviceId).String(),
		Settings:   settings,
	}
}

func (lsa *LeadershipSettingsAccessor) prepareRead(serviceId string) params.Entity {
	return params.Entity{Tag: names.NewServiceTag(serviceId).String()}
}

//
// Bulk calls.
//

func (lsa *LeadershipSettingsAccessor) bulkMerge(args ...params.MergeLeadershipSettingsParam) (*params.ErrorResults, error) {
	// Don't make the jump over the network if we don't have to.
	if len(args) <= 0 {
		return &params.ErrorResults{}, nil
	}

	bulkArgs := params.MergeLeadershipSettingsBulkParams{Params: args}
	var results params.ErrorResults
	return &results, lsa.facadeCaller("Merge", bulkArgs, &results)
}

func (lsa *LeadershipSettingsAccessor) bulkRead(args ...params.Entity) (*params.GetLeadershipSettingsBulkResults, error) {

	// Don't make the jump over the network if we don't have to.
	if len(args) <= 0 {
		return &params.GetLeadershipSettingsBulkResults{}, nil
	}

	bulkArgs := params.Entities{Entities: args}
	var results params.GetLeadershipSettingsBulkResults
	return &results, lsa.facadeCaller("Read", bulkArgs, &results)
}
