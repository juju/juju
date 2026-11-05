// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"

	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

type LXDProfileAPI struct {
	watcherRegistry facade.WatcherRegistry
	accessUnit      common.GetAuthFunc
}

// NewLXDProfileAPI returns a compatibility API for LXD profile methods.
func NewLXDProfileAPI(
	watcherRegistry facade.WatcherRegistry,
	accessUnit common.GetAuthFunc,
) *LXDProfileAPI {
	return &LXDProfileAPI{
		watcherRegistry: watcherRegistry,
		accessUnit:      accessUnit,
	}
}

// NewExternalLXDProfileAPI can be used for API registration.
func NewExternalLXDProfileAPI(
	watcherRegistry facade.WatcherRegistry,
	accessUnit common.GetAuthFunc,
) *LXDProfileAPI {
	return NewLXDProfileAPI(
		watcherRegistry,
		accessUnit,
	)
}

// WatchInstanceData returns a NotifyWatcher for observing
// changes to the lxd profile for one unit.
func (u *LXDProfileAPI) WatchInstanceData(ctx context.Context, args params.Entities) (params.NotifyWatchResults, error) {
	result := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit(ctx)
	if err != nil {
		return params.NotifyWatchResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if !canAccess(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		watcher := watcher.TODO[struct{}]()
		id, _, err := internal.EnsureRegisterWatcher(ctx, u.watcherRegistry, watcher)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		result.Results[i].NotifyWatcherId = id

	}
	return result, nil
}

// LXDProfileName returns the name of the lxd profile applied to the unit's
// machine for the current charm version.
func (u *LXDProfileAPI) LXDProfileName(ctx context.Context, args params.Entities) (params.StringResults, error) {
	return params.StringResults{
		Results: make([]params.StringResult, len(args.Entities)),
	}, nil
}

// CanApplyLXDProfile returns false results. LXD Profiles are not supported.
func (u *LXDProfileAPI) CanApplyLXDProfile(ctx context.Context, args params.Entities) (params.BoolResults, error) {
	result := params.BoolResults{
		Results: make([]params.BoolResult, len(args.Entities)),
	}
	return result, nil
}

// LXDProfileRequired returns false results. LXD profiles are not supported.
func (u *LXDProfileAPI) LXDProfileRequired(ctx context.Context, args params.CharmURLs) (params.BoolResults, error) {
	result := params.BoolResults{
		Results: make([]params.BoolResult, len(args.URLs)),
	}
	return result, nil
}
