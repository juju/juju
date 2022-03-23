// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/permission"
)

// Registry describes the API facades exposed by some API server.
type Registry interface {
	// MustRegister adds a single named facade at a given version to the
	// registry.
	// Factory will be called when someone wants to instantiate an object of
	// this facade, and facadeType defines the concrete type that the returned
	// object will be.
	// The Type information is used to define what methods will be exported in
	// the API, and it must exactly match the actual object returned by the
	// factory.
	MustRegister(string, int, facade.Factory, reflect.Type)
}

// Register is called to expose a package of facades onto a given registry.
func Register(registry Registry) {
	registry.MustRegister("UserManager", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newUserManagerAPI(ctx)
	}, reflect.TypeOf((*UserManagerAPI)(nil)))
	registry.MustRegister("UserManager", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newUserManagerAPI(ctx) // Adds ResetPassword
	}, reflect.TypeOf((*UserManagerAPI)(nil)))
}

// newUserManagerAPI provides the signature required for facade registration.
func newUserManagerAPI(ctx facade.Context) (*UserManagerAPI, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	// Since we know this is a user tag (because AuthClient is true),
	// we just do the type assertion to the UserTag.
	apiUser, _ := authorizer.GetAuthTag().(names.UserTag)
	// Pretty much all of the user manager methods have special casing for admin
	// users, so look once when we start and remember if the user is an admin.
	st := ctx.State()
	isAdmin, err := authorizer.HasPermission(permission.SuperuserAccess, st.ControllerTag())
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &UserManagerAPI{
		state:      st,
		authorizer: authorizer,
		check:      common.NewBlockChecker(st),
		apiUser:    apiUser,
		isAdmin:    isAdmin,
	}, nil
}
