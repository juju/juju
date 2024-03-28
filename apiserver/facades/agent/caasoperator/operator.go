// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import (
	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/unitcommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	k8sspecs "github.com/juju/juju/caas/kubernetes/provider/specs"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/status"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state/watcher"
)

// TODO (manadart 2020-10-21): Remove the ModelUUID method
// from the next version of this facade.

// Facade is the CAAS operator API facade.
type Facade struct {
	auth      facade.Authorizer
	resources facade.Resources
	state     CAASOperatorState
	broker    CAASBrokerInterface
	*common.LifeGetter
	*common.AgentEntityWatcher
	*common.Remover
	*common.ToolsSetter
	*common.APIAddresser

	model Model
}

type CAASBrokerInterface interface {
	WatchContainerStart(appName string, containerName string) (corewatcher.StringsWatcher, error)
}

// NewFacade returns a new CAASOperator facade.
func NewFacade(
	resources facade.Resources,
	authorizer facade.Authorizer,
	ctrlSt CAASControllerState,
	st CAASOperatorState,
	appGetter unitcommon.ApplicationGetter,
	broker CAASBrokerInterface,
	leadershipRevoker leadership.Revoker,
) (*Facade, error) {
	if !authorizer.AuthApplicationAgent() {
		return nil, apiservererrors.ErrPerm
	}
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	canRead := common.AuthAny(
		common.AuthFuncForTagKind(names.ApplicationTagKind),
		common.AuthFuncForTagKind(names.UnitTagKind),
	)
	accessUnit := unitcommon.UnitAccessor(authorizer, appGetter)
	return &Facade{
		LifeGetter:         common.NewLifeGetter(st, canRead),
		APIAddresser:       common.NewAPIAddresser(ctrlSt, resources),
		AgentEntityWatcher: common.NewAgentEntityWatcher(st, resources, canRead),
		Remover:            common.NewRemover(st, common.RevokeLeadershipFunc(leadershipRevoker), true, accessUnit),
		ToolsSetter:        common.NewToolsSetter(st, common.AuthFuncForTag(authorizer.GetAuthTag())),
		auth:               authorizer,
		resources:          resources,
		state:              st,
		model:              model,
		broker:             broker,
	}, nil
}

// CurrentModel returns the name and UUID for the current juju model.
func (f *Facade) CurrentModel() (params.ModelResult, error) {
	return params.ModelResult{
		Name: f.model.Name(),
		UUID: f.model.UUID(),
		Type: string(f.model.Type()),
	}, nil
}

// SetStatus sets the status of each given entity.
func (f *Facade) SetStatus(args params.SetStatus) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	authTag := f.auth.GetAuthTag()
	for i, arg := range args.Entities {
		tag, err := names.ParseApplicationTag(arg.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if tag != authTag {
			results.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		info := status.StatusInfo{
			Status:  status.Status(arg.Status),
			Message: arg.Info,
			Data:    arg.Data,
		}
		results.Results[i].Error = apiservererrors.ServerError(f.setStatus(tag, info))
	}
	return results, nil
}

func (f *Facade) setStatus(tag names.ApplicationTag, info status.StatusInfo) error {
	app, err := f.state.Application(tag.Id())
	if err != nil {
		return errors.Trace(err)
	}
	return app.SetOperatorStatus(info)
}

// Charm returns the charm info for all given applications.
func (f *Facade) Charm(args params.Entities) (params.ApplicationCharmResults, error) {
	results := params.ApplicationCharmResults{
		Results: make([]params.ApplicationCharmResult, len(args.Entities)),
	}
	authTag := f.auth.GetAuthTag()
	for i, entity := range args.Entities {
		tag, err := names.ParseApplicationTag(entity.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if tag != authTag {
			results.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		application, err := f.state.Application(tag.Id())
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		ch, force, err := application.Charm()
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].Result = &params.ApplicationCharm{
			URL:                  ch.URL(),
			ForceUpgrade:         force,
			SHA256:               ch.BundleSha256(),
			CharmModifiedVersion: application.CharmModifiedVersion(),
			DeploymentMode:       string(charm.ModeWorkload),
		}
		if d := ch.Meta().Deployment; d != nil {
			results.Results[i].Result.DeploymentMode = string(d.DeploymentMode)
		}
	}
	return results, nil
}

// SetPodSpec sets the container specs for a set of applications.
// TODO(juju3) - remove
func (f *Facade) SetPodSpec(args params.SetPodSpecParams) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Specs)),
	}

	for i, arg := range args.Specs {
		tag, err := names.ParseApplicationTag(arg.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if !f.auth.AuthOwner(tag) {
			results.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if _, err := k8sspecs.ParsePodSpec(arg.Value); err != nil {
			results.Results[i].Error = apiservererrors.ServerError(errors.New("invalid pod spec"))
			continue
		}
		results.Results[i].Error = apiservererrors.ServerError(
			// NOTE(achilleasa) the CAAS operator is a singleton so
			// we can safely bypass the leadership checks when
			// updating pod specs.
			f.model.SetPodSpec(nil, tag, &arg.Value),
		)
	}
	return results, nil
}

// WatchUnits starts a StringsWatcher to watch changes to the
// lifecycle states of units for the specified applications in
// this model.
func (f *Facade) WatchUnits(args params.Entities) (params.StringsWatchResults, error) {
	results := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		id, changes, err := f.watchUnits(arg.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].StringsWatcherId = id
		results.Results[i].Changes = changes
	}
	return results, nil
}

func (f *Facade) watchUnits(tagString string) (string, []string, error) {
	tag, err := names.ParseApplicationTag(tagString)
	if err != nil {
		return "", nil, errors.Trace(err)
	}
	app, err := f.state.Application(tag.Id())
	if err != nil {
		return "", nil, errors.Trace(err)
	}
	w := app.WatchUnits()
	if changes, ok := <-w.Changes(); ok {
		return f.resources.Register(w), changes, nil
	}
	return "", nil, watcher.EnsureErr(w)
}

// WatchContainerStart starts a StringWatcher to watch for container start events
// on the CAAS api for a specific application and container.
func (f *Facade) WatchContainerStart(args params.WatchContainerStartArgs) (params.StringsWatchResults, error) {
	results := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(args.Args)),
	}
	for i, arg := range args.Args {
		id, changes, err := f.watchContainerStart(arg.Entity.Tag, arg.Container)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].StringsWatcherId = id
		results.Results[i].Changes = changes
	}
	return results, nil
}

func (f *Facade) watchContainerStart(tagString string, containerName string) (string, []string, error) {
	tag, err := names.ParseApplicationTag(tagString)
	if err != nil {
		return "", nil, errors.Trace(err)
	}
	w, err := f.broker.WatchContainerStart(tag.Name, containerName)
	if err != nil {
		return "", nil, errors.Trace(err)
	}
	uw, err := newUnitIDWatcher(f.model, w)
	if err != nil {
		return "", nil, errors.Trace(err)
	}
	if changes, ok := <-uw.Changes(); ok {
		return f.resources.Register(uw), changes, nil
	}
	return "", nil, watcher.EnsureErr(uw)
}

// ModelUUID returns the model UUID that this facade is used to operate.
// It is implemented here directly as a result of removing it from
// embedded APIAddresser *without* bumping the facade version.
// It should be blanked when this facade version is next incremented.
func (f *Facade) ModelUUID() params.StringResult {
	return params.StringResult{Result: f.model.UUID()}
}
