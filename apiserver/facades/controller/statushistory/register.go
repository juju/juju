// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistory

import (
	"reflect"

	"github.com/juju/juju/v2/apiserver/common"
	"github.com/juju/juju/v2/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("StatusHistory", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newAPI(ctx)
	}, reflect.TypeOf((*API)(nil)))
}

// newAPI returns an API Instance.
func newAPI(ctx facade.Context) (*API, error) {
	st := ctx.State()
	m, err := st.Model()
	if err != nil {
		return nil, err
	}

	auth := ctx.Auth()
	return &API{
		ModelWatcher: common.NewModelWatcher(m, ctx.Resources(), auth),
		st:           st,
		authorizer:   auth,
	}, nil
}
