// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
)

type facadeCaller interface {
	FacadeCall(request string, params, response interface{}) error
}

// NewLeadershipSettingsAccessor returns a new LeadershipSettingsAccessor.
func NewLeadershipSettingsAccessor(caller facadeCaller) *LeadershipSettingsAccessor {
	return &LeadershipSettingsAccessor{caller}
}

// LeadershipSettingsAccessor provides a type that can make RPC calls
// to a service which can read, write, and watch leadership settings.
type LeadershipSettingsAccessor struct {
	//base.ClientFacade
	facadeCaller
}

// Merge merges the provided settings into the leadership settings for
// the given service ID. Only leaders of a given service may perform
// this operation.
func (lsa *LeadershipSettingsAccessor) Merge(serviceId string, settings map[string]interface{}) error {
	results, err := lsa.bulkMerge(lsa.prepareMerge(serviceId, settings))
	if err != nil {
		return errors.Annotate(err, "could not merge settings")
	}
	return results.Results[0].Error
}

// Read retrieves the leadership settings for the given service
// ID. Anyone may perform this operation.
func (lsa *LeadershipSettingsAccessor) Read(serviceId string) (map[string]interface{}, error) {
	results, err := lsa.bulkRead(lsa.prepareRead(serviceId))
	if err != nil {
		return nil, errors.Annotate(err, "could not read leadership settings")
	}
	return results.Results[0].Settings, results.Results[0].Error
}

// SettingsChangeNotifier returns a channel which can be used to wait
// for leadership settings changes to be made for a given service ID.
func (lsa *LeadershipSettingsAccessor) SettingsChangeNotifier(serviceId string) <-chan error {
	notifier := make(chan error)
	go func() {
		var results params.ErrorResults
		err := lsa.FacadeCall("BlockUntilChanges", params.LeadershipWatchSettingsParam{names.NewServiceTag(serviceId).String()}, &results)
		if err != nil {
			notifier <- errors.Annotate(err, "could not block on leadership settings changes")
		}
		notifier <- results.Results[0].Error
	}()
	return notifier
}

//
// Prepare functions for building bulk-calls.
//

func (lsa *LeadershipSettingsAccessor) prepareMerge(serviceId string, settings map[string]interface{}) params.MergeLeadershipSettingsParam {
	return params.MergeLeadershipSettingsParam{
		ServiceTag: names.NewServiceTag(serviceId).String(),
		Settings:   settings,
	}
}

func (lsa *LeadershipSettingsAccessor) prepareRead(serviceId string) params.GetLeadershipSettingsParams {
	return params.GetLeadershipSettingsParams{ServiceTag: names.NewServiceTag(serviceId).String()}
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
	return &results, lsa.FacadeCall("Merge", bulkArgs, &results)
}

func (lsa *LeadershipSettingsAccessor) bulkRead(args ...params.GetLeadershipSettingsParams) (*params.GetLeadershipSettingsBulkResults, error) {

	// Don't make the jump over the network if we don't have to.
	if len(args) <= 0 {
		return &params.GetLeadershipSettingsBulkResults{}, nil
	}

	bulkArgs := params.GetLeadershipSettingsBulkParams{Params: args}
	var results params.GetLeadershipSettingsBulkResults
	return &results, lsa.FacadeCall("Read", bulkArgs, &results)
}
