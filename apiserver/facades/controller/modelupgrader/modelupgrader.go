// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelupgrader

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
)

var logger = loggo.GetLogger("juju.apiserver.modelupgrader")

type Facade struct {
	backend       Backend
	pool          Pool
	providers     ProviderRegistry
	entityWatcher EntityWatcher
	statusSetter  StatusSetter
}

// EntityWatcher is an interface that provides a means of watching
// entities.
type EntityWatcher interface {
	Watch(params.Entities) (params.NotifyWatchResults, error)
}

// ProviderRegistry provides the subset of environs.ProviderRegistry
// that we require.
type ProviderRegistry interface {
	Provider(string) (environs.EnvironProvider, error)
}

// StatusSetter is an interface that provides a means of setting
// the status of entities.
type StatusSetter interface {
	SetStatus(params.SetStatus) (params.ErrorResults, error)
}

// NewStateFacade provides the signature required for facade registration.
func NewStateFacade(ctx facade.Context) (*Facade, error) {
	pool := NewPool(ctx.StatePool())
	registry := environs.GlobalProviderRegistry()
	watcher := common.NewAgentEntityWatcher(
		ctx.State(),
		ctx.Resources(),
		common.AuthFuncForTagKind(names.ModelTagKind),
	)
	statusSetter := common.NewStatusSetter(
		ctx.State(),
		common.AuthFuncForTagKind(names.ModelTagKind),
	)
	return NewFacade(ctx.State(), pool, registry, watcher, statusSetter, ctx.Auth())
}

// NewFacade returns a new Facade using the given Backend and Authorizer.
func NewFacade(
	backend Backend,
	pool Pool,
	providers ProviderRegistry,
	entityWatcher EntityWatcher,
	statusSetter StatusSetter,
	auth facade.Authorizer,
) (*Facade, error) {
	if !auth.AuthController() {
		return nil, common.ErrPerm
	}
	return &Facade{
		backend:       backend,
		pool:          pool,
		providers:     providers,
		entityWatcher: entityWatcher,
		statusSetter:  statusSetter,
	}, nil
}

// ModelEnvironVersion returns the current version of the environ corresponding
// to each specified model.
func (f *Facade) ModelEnvironVersion(args params.Entities) (params.IntResults, error) {
	result := params.IntResults{
		Results: make([]params.IntResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		v, err := f.modelEnvironVersion(arg)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		result.Results[i].Result = v
	}
	return result, nil
}

func (f *Facade) modelEnvironVersion(arg params.Entity) (int, error) {
	tag, err := names.ParseModelTag(arg.Tag)
	if err != nil {
		return -1, errors.Trace(err)
	}
	model, release, err := f.pool.GetModel(tag.Id())
	if err != nil {
		return -1, errors.Trace(err)
	}
	defer release()
	return model.EnvironVersion(), nil
}

// ModelTargetEnvironVersion returns the target version of the environ
// corresponding to each specified model. The target version is the
// environ provider's version.
func (f *Facade) ModelTargetEnvironVersion(args params.Entities) (params.IntResults, error) {
	result := params.IntResults{
		Results: make([]params.IntResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		v, err := f.modelTargetEnvironVersion(arg)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		result.Results[i].Result = v
	}
	return result, nil
}

func (f *Facade) modelTargetEnvironVersion(arg params.Entity) (int, error) {
	tag, err := names.ParseModelTag(arg.Tag)
	if err != nil {
		return -1, errors.Trace(err)
	}
	model, release, err := f.pool.GetModel(tag.Id())
	if err != nil {
		return -1, errors.Trace(err)
	}
	defer release()
	cloud, err := f.backend.Cloud(model.CloudName())
	if err != nil {
		return -1, errors.Trace(err)
	}
	provider, err := f.providers.Provider(cloud.Type)
	if err != nil {
		return -1, errors.Trace(err)
	}
	return provider.Version(), nil
}

// SetModelEnvironVersion sets the current version of the environ corresponding
// to each specified model.
func (f *Facade) SetModelEnvironVersion(args params.SetModelEnvironVersions) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Models)),
	}
	for i, arg := range args.Models {
		err := f.setModelEnvironVersion(arg)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
		}
	}
	return result, nil
}

func (f *Facade) setModelEnvironVersion(arg params.SetModelEnvironVersion) error {
	tag, err := names.ParseModelTag(arg.ModelTag)
	if err != nil {
		return errors.Trace(err)
	}
	model, release, err := f.pool.GetModel(tag.Id())
	if err != nil {
		return errors.Trace(err)
	}
	defer release()
	return errors.Trace(model.SetEnvironVersion(arg.Version))
}

// WatchModelEnvironVersion watches for changes to the environ version of the
// specified models.
//
// NOTE(axw) this is currently implemented in terms of state.Model.Watch, so
// the client may be notified of changes unrelated to the environ version.
func (f *Facade) WatchModelEnvironVersion(args params.Entities) (params.NotifyWatchResults, error) {
	return f.entityWatcher.Watch(args)
}

// SetModelStatus sets the status of each given model.
func (f *Facade) SetModelStatus(args params.SetStatus) (params.ErrorResults, error) {
	return f.statusSetter.SetStatus(args)
}
