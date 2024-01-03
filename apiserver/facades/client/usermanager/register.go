// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/permission"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("UserManager", 3, func(ctx facade.Context) (facade.Facade, error) {
		return newUserManagerAPI(ctx) // Adds ModelUserInfo
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
	err := authorizer.HasPermission(permission.SuperuserAccess, st.ControllerTag())
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return nil, errors.Trace(err)
	}
	isAdmin := err == nil

	return &UserManagerAPI{
		state:      st,
		pool:       ctx.StatePool(),
		authorizer: authorizer,
		check:      common.NewBlockChecker(st),
		apiUser:    apiUser,
		isAdmin:    isAdmin,
		logger:     ctx.Logger().Child("usermanager"),
	}, nil
}
