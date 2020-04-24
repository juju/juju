// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package externalcontrollerupdater

import (
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

var logger = loggo.GetLogger("juju.apiserver.externalcontrollerupdater")

// ExternalControllerUpdaterAPI provides access to the CrossModelRelations API facade.
type ExternalControllerUpdaterAPI struct {
	externalControllers state.ExternalControllers
	resources           facade.Resources
}

// NewStateAPI creates a new server-side CrossModelRelationsAPI API facade
// backed by global state.
func NewStateAPI(ctx facade.Context) (*ExternalControllerUpdaterAPI, error) {
	return NewAPI(
		ctx.Auth(),
		ctx.Resources(),
		state.NewExternalControllers(ctx.State()),
	)
}

// NewAPI creates a new server-side CrossModelRelationsAPI API facade backed
// by the given interfaces.
func NewAPI(
	auth facade.Authorizer,
	resources facade.Resources,
	externalControllers state.ExternalControllers,
) (*ExternalControllerUpdaterAPI, error) {
	if !auth.AuthController() {
		return nil, common.ErrPerm
	}
	return &ExternalControllerUpdaterAPI{
		externalControllers,
		resources,
	}, nil
}

// WatchExternalControllers watches for the addition and removal of external
// controller records to the local controller's database.
func (api *ExternalControllerUpdaterAPI) WatchExternalControllers() (params.StringsWatchResults, error) {
	w := api.externalControllers.Watch()
	changes, ok := <-w.Changes()
	if !ok {
		return params.StringsWatchResults{
			[]params.StringsWatchResult{{
				Error: common.ServerError(watcher.EnsureErr(w)),
			}},
		}, nil
	}
	return params.StringsWatchResults{
		[]params.StringsWatchResult{{
			StringsWatcherId: api.resources.Register(w),
			Changes:          changes,
		}},
	}, nil
}

// ExternalControllerInfo returns the info for the specified external controllers.
func (s *ExternalControllerUpdaterAPI) ExternalControllerInfo(args params.Entities) (params.ExternalControllerInfoResults, error) {
	result := params.ExternalControllerInfoResults{
		Results: make([]params.ExternalControllerInfoResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		controllerTag, err := names.ParseControllerTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		controller, err := s.externalControllers.Controller(controllerTag.Id())
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		info := controller.ControllerInfo()
		result.Results[i].Result = &params.ExternalControllerInfo{
			ControllerTag: controllerTag.String(),
			Alias:         info.Alias,
			Addrs:         info.Addrs,
			CACert:        info.CACert,
		}
	}
	return result, nil
}

// SetExternalControllerInfo saves the info for the specified external controllers.
func (s *ExternalControllerUpdaterAPI) SetExternalControllerInfo(args params.SetExternalControllersInfoParams) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Controllers)),
	}
	for i, arg := range args.Controllers {
		controllerTag, err := names.ParseControllerTag(arg.Info.ControllerTag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		if _, err := s.externalControllers.Save(crossmodel.ControllerInfo{
			ControllerTag: controllerTag,
			Alias:         arg.Info.Alias,
			Addrs:         arg.Info.Addrs,
			CACert:        arg.Info.CACert,
		}); err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
	}
	return result, nil
}
