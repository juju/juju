// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"

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
		return err
	}

	results, err := lsa.bulkMerge(lsa.prepareMerge(serviceId, settings))
	if err != nil {
		return errors.Annotate(err, "could not merge settings")
	}
	return results.Results[0].Error
}

// Read retrieves the leadership settings for the given service
// ID. Anyone may perform this operation.
func (lsa *LeadershipSettingsAccessor) Read(serviceId string) (map[string]string, error) {

	if err := lsa.checkApiVersion("Read"); err != nil {
		return nil, err
	}

	results, err := lsa.bulkRead(lsa.prepareRead(serviceId))
	if err != nil {
		return nil, errors.Annotate(err, "could not read leadership settings")
	}
	return results.Results[0].Settings, results.Results[0].Error
}

// WatchLeadershipSettings returns a watcher which can be used to wait
// for leadership settings changes to be made for a given service ID.
func (lsa *LeadershipSettingsAccessor) WatchLeadershipSettings(serviceId string) (watcher.NotifyWatcher, error) {

	if err := lsa.checkApiVersion("WatchLeadershipSettings"); err != nil {
		return nil, err
	}

	var results params.NotifyWatchResults
	if err := lsa.facadeCaller(
		"WatchLeadershipSettings",
		params.Entities{[]params.Entity{{names.NewServiceTag(serviceId).String()}}},
		&results,
	); err != nil {
		return nil, errors.Annotate(err, "could not watch leadership settings")
	}
	fmt.Printf("%v", results)
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
