// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agenttools

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state/stateenvirons"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("AgentTools", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newFacade(ctx)
	}, reflect.TypeOf((*AgentToolsAPI)(nil)))
}

// newFacade is used to register the facade.
func newFacade(ctx facade.Context) (*AgentToolsAPI, error) {
	st := ctx.State()
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	newEnviron := func() (environs.Environ, error) {
		newEnviron := stateenvirons.GetNewEnvironFunc(environs.New)
		return newEnviron(model)
	}
	return NewAgentToolsAPI(st, newEnviron, findTools, envVersionUpdate, ctx.Auth())
}
