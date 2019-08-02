// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	k8sprovider "github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state/watcher"
)

var logger = loggo.GetLogger("juju.apiserver.caasoperator")

// Facade provides access to the CAASOperator facade.
type Facade struct {
	auth      facade.Authorizer
	resources facade.Resources
	state     CAASOperatorState
	*common.LifeGetter
	*common.AgentEntityWatcher
	*common.Remover
	*common.ToolsSetter
	*common.APIAddresser

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

	accessUnit := func() (common.AuthFunc, error) {
		return getAccessUnitChecker(st, authorizer.GetAuthTag())
	}
	return &Facade{
		LifeGetter:         common.NewLifeGetter(st, canRead),
		APIAddresser:       common.NewAPIAddresser(st, resources),
		AgentEntityWatcher: common.NewAgentEntityWatcher(st, resources, canRead),
		Remover:            common.NewRemover(st, true, accessUnit),
		ToolsSetter:        common.NewToolsSetter(st, common.AuthFuncForTag(authorizer.GetAuthTag())),
		auth:               authorizer,
		resources:          resources,
		state:              st,
		model:              model,
	}, nil
}

func getAccessUnitChecker(st CAASOperatorState, authTag names.Tag) (common.AuthFunc, error) {
	switch tag := authTag.(type) {
	case names.ApplicationTag:
		// Any of the units belonging to
		// the application can be accessed.
		app, err := st.Application(tag.Name)
		if err != nil {
			return nil, errors.Trace(err)
		}
		allUnits, err := app.AllUnits()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return func(tag names.Tag) bool {
			for _, u := range allUnits {
				if u.Tag() == tag {
					return true
				}
			}
			return false
		}, nil
	default:
		return nil, errors.Errorf("expected names.ApplicationTag, got %T", tag)
	}
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
		if _, err := k8sprovider.ParsePodSpec(arg.Value); err != nil {
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

// UnitsStatus returns units' status.
func (f *Facade) UnitsStatus(args params.Entities) (params.UnitStatusResults, error) {
	results := params.UnitStatusResults{
		Results: make([]params.UnitStatusResult, len(args.Entities)),
	}

	canAccessUnit, err := getAccessUnitChecker(f.state, f.auth.GetAuthTag())
	if err != nil {
		return results, errors.Trace(err)
	}
	for i, entity := range args.Entities {
		unitTag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		if !canAccessUnit(unitTag) {
			results.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		unit, err := f.state.Unit(unitTag.Id())
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		unitStatus := &params.UnitStatus{}
		container, err := unit.ContainerInfo()
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		addr := container.Address()
		if addr != nil {
			unitStatus.Address = addr.Value
		}
		unitStatus.ProviderId = container.ProviderId()

		unitPorts, _ := unit.OpenedPorts()
		for _, port := range unitPorts {
			unitStatus.OpenedPorts = append(unitStatus.OpenedPorts, port.String())
		}
		curl, _ := unit.CharmURL()
		if curl != nil {
			unitStatus.Charm = curl.String()
		}
		results.Results[i].Result = unitStatus
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
