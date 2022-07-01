// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemanager

import (
	"reflect"

	"github.com/juju/juju/v3/apiserver/common"
	apiservererrors "github.com/juju/juju/v3/apiserver/errors"
	"github.com/juju/juju/v3/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("ImageManager", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newImageManagerAPI(ctx)
	}, reflect.TypeOf((*ImageManagerAPI)(nil)))
}

// newImageManagerAPI creates a new server-side imagemanager API end point.
func newImageManagerAPI(ctx facade.Context) (*ImageManagerAPI, error) {
	// Only clients can access the image manager service.
	authorizer := ctx.Auth()
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}
	st := ctx.State()
	return &ImageManagerAPI{
		state:      getState(st),
		resources:  ctx.Resources(),
		authorizer: authorizer,
		check:      common.NewBlockChecker(st),
	}, nil
}
