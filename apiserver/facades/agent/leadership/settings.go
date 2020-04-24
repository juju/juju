// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/leadership"
)

// NewLeadershipSettingsAccessor creates a new
// LeadershipSettingsAccessor.
func NewLeadershipSettingsAccessor(
	authorizer facade.Authorizer,
	registerWatcherFn RegisterWatcherFn,
	getSettingsFn GetSettingsFn,
	leaderCheckFn LeaderCheckFn,
	mergeSettingsChunkFn MergeSettingsChunkFn,
) *LeadershipSettingsAccessor {

	return &LeadershipSettingsAccessor{
		authorizer:           authorizer,
		registerWatcherFn:    registerWatcherFn,
		getSettingsFn:        getSettingsFn,
		leaderCheckFn:        leaderCheckFn,
		mergeSettingsChunkFn: mergeSettingsChunkFn,
	}
}

// SettingsChangeNotifierFn declares a function-type which will return
// a channel that can be blocked on to be notified of setting changes
// for the provided document key.
type RegisterWatcherFn func(serviceId string) (watcherId string, _ error)

// GetSettingsFn declares a function-type which will return leadership
// settings for the given service ID.
type GetSettingsFn func(serviceId string) (map[string]string, error)

// LeaderCheckFn returns a Token whose Check method will return an error
// if the unit is not leader of the service.
type LeaderCheckFn func(serviceId, unitId string) leadership.Token

// MergeSettingsChunk declares a function-type which will write the
// provided settings chunk into the greater leadership settings for
// the provided service ID, so long as the supplied Token remains
// valid.
type MergeSettingsChunkFn func(token leadership.Token, serviceId string, settings map[string]string) error

// LeadershipSettingsAccessor provides a type which can read, write,
// and watch leadership settings.
type LeadershipSettingsAccessor struct {
	authorizer           facade.Authorizer
	registerWatcherFn    RegisterWatcherFn
	getSettingsFn        GetSettingsFn
	leaderCheckFn        LeaderCheckFn
	mergeSettingsChunkFn MergeSettingsChunkFn
}

func (lsa *LeadershipSettingsAccessor) callerApplication() (string, error) {
	var appName string
	switch authTag := lsa.authorizer.GetAuthTag().(type) {
	case names.UnitTag:
		var err error
		appName, err = names.UnitApplication(authTag.Id())
		if err != nil {
			return "", err
		}
	case names.ApplicationTag:
		appName = authTag.Id()
	default:
		return "", errors.Errorf("invalid auth tag type %T: %v", authTag, authTag.String())
	}
	return appName, nil
}

// Merge merges in the provided leadership settings. Only leaders for
// the given service may perform this operation.
func (lsa *LeadershipSettingsAccessor) Merge(bulkArgs params.MergeLeadershipSettingsBulkParams) (params.ErrorResults, error) {

	requireAppName, err := lsa.callerApplication()
	if err != nil {
		return params.ErrorResults{}, err
	}
	// Start out assuming the caller is a unit (for older clients).
	callerUnitId := lsa.authorizer.GetAuthTag().Id()

	results := make([]params.ErrorResult, len(bulkArgs.Params))

	for i, arg := range bulkArgs.Params {
		result := &results[i]

		// TODO(fwereade): we shoudn't assume a ApplicationTag: we should
		// use an actual auth func to determine permissions.
		applicationTag, err := names.ParseApplicationTag(arg.ApplicationTag)
		if err != nil {
			result.Error = common.ServerError(err)
			continue
		}

		// If a unit is passed in as an arg, use that instead of the caller id.
		if arg.UnitTag != "" {
			unitTag, err := names.ParseUnitTag(arg.UnitTag)
			if err != nil {
				result.Error = common.ServerError(err)
				continue
			}
			callerUnitId = unitTag.Id()
			unitAppName, err := names.UnitApplication(callerUnitId)
			if err != nil || unitAppName != requireAppName {
				result.Error = common.ServerError(common.ErrPerm)
				continue
			}
		}

		appName := applicationTag.Id()
		if appName != requireAppName {
			result.Error = common.ServerError(common.ErrPerm)
			continue
		}

		token := lsa.leaderCheckFn(appName, callerUnitId)
		err = lsa.mergeSettingsChunkFn(token, appName, arg.Settings)
		if err != nil {
			result.Error = common.ServerError(err)
		}
	}

	return params.ErrorResults{Results: results}, nil
}

// Read reads leadership settings for the provided service ID. Any
// unit of the service may perform this operation.
func (lsa *LeadershipSettingsAccessor) Read(bulkArgs params.Entities) (params.GetLeadershipSettingsBulkResults, error) {

	requireAppName, err := lsa.callerApplication()
	if err != nil {
		return params.GetLeadershipSettingsBulkResults{}, err
	}
	results := make([]params.GetLeadershipSettingsResult, len(bulkArgs.Entities))

	for i, arg := range bulkArgs.Entities {
		result := &results[i]

		// TODO(fwereade): we shoudn't assume a ApplicationTag: we should
		// use an actual auth func to determine permissions.
		applicationTag, err := names.ParseApplicationTag(arg.Tag)
		if err != nil {
			result.Error = common.ServerError(err)
			continue
		}

		appName := applicationTag.Id()
		if appName != requireAppName {
			result.Error = common.ServerError(common.ErrPerm)
			continue
		}

		settings, err := lsa.getSettingsFn(appName)
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
func (lsa *LeadershipSettingsAccessor) WatchLeadershipSettings(bulkArgs params.Entities) (params.NotifyWatchResults, error) {

	requireAppName, err := lsa.callerApplication()
	if err != nil {
		return params.NotifyWatchResults{}, err
	}
	results := make([]params.NotifyWatchResult, len(bulkArgs.Entities))

	for i, arg := range bulkArgs.Entities {
		result := &results[i]

		// TODO(fwereade): we shoudn't assume a ApplicationTag: we should
		// use an actual auth func to determine permissions.
		applicationTag, err := names.ParseApplicationTag(arg.Tag)
		if err != nil {
			result.Error = common.ServerError(err)
			continue
		}

		appName := applicationTag.Id()
		if appName != requireAppName {
			result.Error = common.ServerError(common.ErrPerm)
			continue
		}

		watcherId, err := lsa.registerWatcherFn(appName)
		if err != nil {
			result.Error = common.ServerError(err)
			continue
		}

		result.NotifyWatcherId = watcherId
	}
	return params.NotifyWatchResults{Results: results}, nil
}
