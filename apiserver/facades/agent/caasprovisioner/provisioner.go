// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasprovisioner

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/watcher"
)

type API struct {
	auth      facade.Authorizer
	resources facade.Resources

	state     CAASProvisionerState
	caasModel CAASModel
}

// NewStateCAASProvisionerAPI provides the signature required for facade registration.
func NewStateCAASProvisionerAPI(ctx facade.Context) (*API, error) {

	authorizer := ctx.Auth()
	if !authorizer.AuthMachineAgent() && !authorizer.AuthController() {
		return nil, common.ErrPerm
	}

	resources := ctx.Resources()
	return NewCAASProvisionerAPI(resources, authorizer, stateShim{ctx.State()})
}

// NewCAASProvisionerAPI returns a new CAASProvisionerAPI facade.
func NewCAASProvisionerAPI(
	resources facade.Resources,
	authorizer facade.Authorizer,
	st CAASProvisionerState,
) (*API, error) {
	caasModel, err := st.CAASModel()
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &API{
		auth:      authorizer,
		resources: resources,
		caasModel: caasModel,
		state:     st,
	}, nil
}

// ConnectionConfig returns the configuration to be used
// when provisioning applications.
func (a *API) ConnectionConfig() (params.CAASConnectionConfig, error) {
	cfg, err := a.caasModel.ConnectionConfig()
	if err != nil {
		return params.CAASConnectionConfig{}, common.ServerError(err)
	}
	return params.CAASConnectionConfig{
		Endpoint:       cfg.Endpoint,
		Username:       cfg.Username,
		Password:       cfg.Password,
		CACertificates: cfg.CACertificates,
		CertData:       cfg.CertData,
		KeyData:        cfg.KeyData,
	}, nil
}

// WatchApplications starts a StringsWatcher to watch CAAS applications
// deployed to this model.
func (a *API) WatchApplications() (params.StringsWatchResult, error) {
	watch := a.state.WatchApplications()
	// Consume the initial event and forward it to the result.
	if changes, ok := <-watch.Changes(); ok {
		return params.StringsWatchResult{
			StringsWatcherId: a.resources.Register(watch),
			Changes:          changes,
		}, nil
	}
	return params.StringsWatchResult{}, watcher.EnsureErr(watch)
}
