// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/status"
)

type Facade struct {
	auth      facade.Authorizer
	resources facade.Resources
	state     CAASOperatorState
	*common.LifeGetter
	*common.AgentEntityWatcher
	*common.Remover

	model Model
}

// NewStateFacade provides the signature required for facade registration.
func NewStateFacade(ctx facade.Context) (*Facade, error) {
	authorizer := ctx.Auth()
	resources := ctx.Resources()
	return NewFacade(resources, authorizer, stateShim{ctx.State()})
}

// NewFacade returns a new CAASOperator facade.
func NewFacade(
	resources facade.Resources,
	authorizer facade.Authorizer,
	st CAASOperatorState,
) (*Facade, error) {
	if !authorizer.AuthApplicationAgent() {
		return nil, common.ErrPerm
	}
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	canRead := common.AuthAny(
		common.AuthFuncForTagKind(names.ApplicationTagKind),
		common.AuthFuncForTagKind(names.UnitTagKind),
	)
	canWrite := canRead
	return &Facade{
		LifeGetter:         common.NewLifeGetter(st, canRead),
		AgentEntityWatcher: common.NewAgentEntityWatcher(st, resources, canRead),
		Remover:            common.NewRemover(st, true, canWrite),
		auth:               authorizer,
		resources:          resources,
		state:              st,
		model:              model,
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
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		if tag != authTag {
			results.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		info := status.StatusInfo{
			Status:  status.Status(arg.Status),
			Message: arg.Info,
			Data:    arg.Data,
		}
		results.Results[i].Error = common.ServerError(f.setStatus(tag, info))
	}
	return results, nil
}

func (f *Facade) setStatus(tag names.ApplicationTag, info status.StatusInfo) error {
	app, err := f.state.Application(tag.Id())
	if err != nil {
		return errors.Trace(err)
	}
	return app.SetStatus(info)
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
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		if tag != authTag {
			results.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		application, err := f.state.Application(tag.Id())
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		charm, force, err := application.Charm()
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		results.Results[i].Result = &params.ApplicationCharm{
			URL:                  charm.URL().String(),
			ForceUpgrade:         force,
			SHA256:               charm.BundleSha256(),
			CharmModifiedVersion: application.CharmModifiedVersion(),
		}
	}
	return results, nil
}

// SetPodSpec sets the container specs for a set of applications.
func (f *Facade) SetPodSpec(args params.SetPodSpecParams) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Specs)),
	}

	cfg, err := f.model.ModelConfig()
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	provider, err := environs.Provider(cfg.Type())
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	caasProvider, ok := provider.(caas.ContainerEnvironProvider)
	if !ok {
		return params.ErrorResults{}, errors.NotValidf("container environ provider %T", provider)
	}

	for i, arg := range args.Specs {
		tag, err := names.ParseApplicationTag(arg.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		if !f.auth.AuthOwner(tag) {
			results.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		if _, err := caasProvider.ParsePodSpec(arg.Value); err != nil {
			results.Results[i].Error = common.ServerError(errors.New("invalid pod spec"))
			continue
		}
		results.Results[i].Error = common.ServerError(
			f.model.SetPodSpec(tag, arg.Value),
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
			results.Results[i].Error = common.ServerError(err)
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
