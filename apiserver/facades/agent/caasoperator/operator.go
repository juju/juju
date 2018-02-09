// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import (
	"github.com/juju/errors"
	"github.com/juju/utils/proxy"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/status"
)

type Facade struct {
	auth      facade.Authorizer
	resources facade.Resources
	state     CAASOperatorState
	*common.LifeGetter

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
	return &Facade{
		LifeGetter: common.NewLifeGetter(
			st, common.AuthAny(
				common.AuthFuncForTagKind(names.ApplicationTagKind),
				common.AuthFuncForTagKind(names.UnitTagKind),
			),
		),
		auth:      authorizer,
		resources: resources,
		state:     st,
		model:     model,
	}, nil
}

// ModelName returns the name of the model.
func (f *Facade) ModelName() (params.StringResult, error) {
	return params.StringResult{Result: f.model.Name()}, nil
}

// APIAddresses returns the API addresses of the controller.
func (f *Facade) APIAddresses() (params.StringsResult, error) {
	addrs, err := apiAddresses(f.state)
	if err != nil {
		return params.StringsResult{}, err
	}
	return params.StringsResult{
		Result: addrs,
	}, nil
}

func apiAddresses(getter CAASOperatorState) ([]string, error) {
	apiHostPorts, err := getter.APIHostPortsForAgents()
	if err != nil {
		return nil, err
	}
	var addrs = make([]string, 0, len(apiHostPorts))
	for _, hostPorts := range apiHostPorts {
		ordered := network.PrioritizeInternalHostPorts(hostPorts, false)
		for _, addr := range ordered {
			if addr != "" {
				addrs = append(addrs, addr)
			}
		}
	}
	return addrs, nil
}

// ProxyConfig returns the proxy config for the current model.
func (f *Facade) ProxyConfig() (params.ProxyConfig, error) {
	var result params.ProxyConfig
	cfg, err := f.model.Config()
	if err != nil {
		return result, err
	}

	proxySettings := cfg.ProxySettings()
	result = proxyUtilsSettingsToProxySettingsParam(proxySettings)
	return result, nil

}

func proxyUtilsSettingsToProxySettingsParam(settings proxy.Settings) params.ProxyConfig {
	return params.ProxyConfig{
		HTTP:    settings.Http,
		HTTPS:   settings.Https,
		FTP:     settings.Ftp,
		NoProxy: settings.FullNoProxy(),
	}
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
			URL:          charm.URL().String(),
			ForceUpgrade: force,
			SHA256:       charm.BundleSha256(),
		}
	}
	return results, nil
}

// SetContainerSpec sets the container specs for a set of entities.
func (f *Facade) SetContainerSpec(args params.SetContainerSpecParams) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	authTag := f.auth.GetAuthTag()
	canAccess := func(tag names.Tag) bool {
		if tag == authTag {
			return true
		}
		if tag, ok := tag.(names.UnitTag); ok {
			appName, err := names.UnitApplication(tag.Id())
			if err == nil && appName == authTag.Id() {
				return true
			}
		}
		return false
	}
	for i, arg := range args.Entities {
		tag, err := names.ParseTag(arg.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		if !canAccess(tag) {
			results.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		if _, err := caas.ParseContainerSpec(arg.Value); err != nil {
			results.Results[i].Error = common.ServerError(errors.New("invalid container spec"))
			continue
		}
		results.Results[i].Error = common.ServerError(
			f.model.SetContainerSpec(tag, arg.Value),
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
