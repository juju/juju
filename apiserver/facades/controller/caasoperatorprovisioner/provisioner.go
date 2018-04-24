// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorprovisioner

import (
	"fmt"

	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/version"
)

type API struct {
	*common.PasswordChanger
	*common.LifeGetter

	auth      facade.Authorizer
	resources facade.Resources

	state CAASOperatorProvisionerState
}

// NewStateCAASOperatorProvisionerAPI provides the signature required for facade registration.
func NewStateCAASOperatorProvisionerAPI(ctx facade.Context) (*API, error) {

	authorizer := ctx.Auth()
	resources := ctx.Resources()
	return NewCAASOperatorProvisionerAPI(resources, authorizer, ctx.State())
}

// NewCAASOperatorProvisionerAPI returns a new CAAS operator provisioner API facade.
func NewCAASOperatorProvisionerAPI(
	resources facade.Resources,
	authorizer facade.Authorizer,
	st CAASOperatorProvisionerState,
) (*API, error) {
	if !authorizer.AuthController() {
		return nil, common.ErrPerm
	}
	return &API{
		PasswordChanger: common.NewPasswordChanger(st, common.AuthFuncForTagKind(names.ApplicationTagKind)),
		LifeGetter:      common.NewLifeGetter(st, common.AuthFuncForTagKind(names.ApplicationTagKind)),
		auth:            authorizer,
		resources:       resources,
		state:           st,
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

// OperatorProvisioningInfo returns the info needed to provision an operator.
func (a *API) OperatorProvisioningInfo() (params.OperatorProvisioningInfo, error) {
	cfg, err := a.state.ControllerConfig()
	if err != nil {
		return params.OperatorProvisioningInfo{}, err
	}

	imagePath := cfg.CAASOperatorImagePath()
	if imagePath == "" {
		vers := version.Current
		vers.Build = 0
		imagePath = fmt.Sprintf("%s/caas-jujud-operator:%s", "jujusolutions", vers.String())
	}

	return params.OperatorProvisioningInfo{
		ImagePath: imagePath,
	}, nil
}
