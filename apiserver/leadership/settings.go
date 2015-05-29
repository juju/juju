// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
)

// NewLeadershipSettingsAccessor creates a new
// LeadershipSettingsAccessor.
func NewLeadershipSettingsAccessor(
	authorizer common.Authorizer,
	registerWatcherFn RegisterWatcherFn,
	getSettingsFn GetSettingsFn,
	mergeSettingsChunkFn MergeSettingsChunkFn,
	isLeaderFn IsLeaderFn,
) *LeadershipSettingsAccessor {

	return &LeadershipSettingsAccessor{
		authorizer:           authorizer,
		registerWatcherFn:    registerWatcherFn,
		getSettingsFn:        getSettingsFn,
		mergeSettingsChunkFn: mergeSettingsChunkFn,
		isLeaderFn:           isLeaderFn,
	}
}

// SettingsChangeNotifierFn declares a function-type which will return
// a channel that can be blocked on to be notified of setting changes
// for the provided document key.
type RegisterWatcherFn func(serviceId string) (watcherId string, _ error)

// GetSettingsFn declares a function-type which will return leadership
// settings for the given service ID.
type GetSettingsFn func(serviceId string) (map[string]string, error)

// MergeSettingsChunk declares a function-type which will write the
// provided settings chunk into the greater leadership settings for
// the provided service ID.
type MergeSettingsChunkFn func(serviceId string, settings map[string]string) error

// IsLeaderFn declares a function-type which will return whether the
// given service-unit-id combination is currently the leader.
type IsLeaderFn func(serviceId, unitId string) (bool, error)

// LeadershipSettingsAccessor provides a type which can read, write,
// and watch leadership settings.
type LeadershipSettingsAccessor struct {
	authorizer           common.Authorizer
	registerWatcherFn    RegisterWatcherFn
	getSettingsFn        GetSettingsFn
	mergeSettingsChunkFn MergeSettingsChunkFn
	isLeaderFn           IsLeaderFn
}

// Merge merges in the provided leadership settings. Only leaders for
// the given service may perform this operation.
func (lsa *LeadershipSettingsAccessor) Merge(bulkArgs params.MergeLeadershipSettingsBulkParams) (params.ErrorResults, error) {

	callerUnitId := lsa.authorizer.GetAuthTag().Id()
	errors := make([]params.ErrorResult, len(bulkArgs.Params))

	for argIdx, arg := range bulkArgs.Params {

		currErr := &errors[argIdx]
		serviceTag, parseErr := parseServiceTag(arg.ServiceTag)
		if parseErr != nil {
			currErr.Error = parseErr
			continue
		}

		// Check to ensure we can write settings.
		isLeader, err := lsa.isLeaderFn(serviceTag.Id(), callerUnitId)
		if err != nil {
			currErr.Error = common.ServerError(err)
			continue
		}
		if !isLeader || !lsa.authorizer.AuthUnitAgent() {
			currErr.Error = common.ServerError(common.ErrPerm)
			continue
		}

		// TODO(katco-): <2015-01-21 Wed>
		// There is a race-condition here: if this unit should lose
		// leadership status between the check above, and actually
		// writing the settings, another unit could obtain leadership,
		// write settings, and then those settings could be
		// overwritten by this request. This will be addressed in a
		// future PR.

		err = lsa.mergeSettingsChunkFn(serviceTag.Id(), arg.Settings)
		if err != nil {
			currErr.Error = common.ServerError(err)
		}
	}

	return params.ErrorResults{Results: errors}, nil
}

// Read reads leadership settings for the provided service ID. Any
// unit of the service may perform this operation.
func (lsa *LeadershipSettingsAccessor) Read(bulkArgs params.Entities) (params.GetLeadershipSettingsBulkResults, error) {

	results := make([]params.GetLeadershipSettingsResult, len(bulkArgs.Entities))
	for argIdx, arg := range bulkArgs.Entities {

		result := &results[argIdx]

		serviceTag, parseErr := parseServiceTag(arg.Tag)
		if parseErr != nil {
			result.Error = parseErr
			continue
		}

		if !lsa.authorizer.AuthUnitAgent() {
			result.Error = common.ServerError(common.ErrPerm)
			continue
		}

		settings, err := lsa.getSettingsFn(serviceTag.Id())
		if err != nil {
			result.Error = common.ServerError(err)
			continue
		}

		result.Settings = settings
	}

	return params.GetLeadershipSettingsBulkResults{results}, nil
}

// WatchLeadershipSettings will block the caller until leadership settings
// for the given service ID change.
func (lsa *LeadershipSettingsAccessor) WatchLeadershipSettings(arg params.Entities) (params.NotifyWatchResults, error) {

	results := make([]params.NotifyWatchResult, len(arg.Entities))
	for entIdx, entity := range arg.Entities {
		result := &results[entIdx]

		serviceTag, parseErr := parseServiceTag(entity.Tag)
		if parseErr != nil {
			result.Error = parseErr
			continue
		}

		watcherId, err := lsa.registerWatcherFn(serviceTag.Id())
		if err != nil {
			result.Error = common.ServerError(err)
			continue
		}

		result.NotifyWatcherId = watcherId
	}
	return params.NotifyWatchResults{Results: results}, nil
}

// parseServiceTag attempts to parse the given serviceTag, and if it
// fails returns an error which is safe to return to the client -- in
// both a structure and security context.
func parseServiceTag(serviceTag string) (names.ServiceTag, *params.Error) {
	parsedTag, err := names.ParseServiceTag(serviceTag)
	if err != nil {
		// We intentionally mask the real error for security purposes.
		return names.ServiceTag{}, common.ServerError(common.ErrPerm)
	}
	return parsedTag, nil
}
