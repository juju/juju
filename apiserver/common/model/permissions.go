// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"errors"

	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/permission"
)

// HasModelAdmin reports whether a user has admin access to the input model.
// A user has model access if they are a controller superuser,
// or if they have been explicitly granted admin access to the model.
func HasModelAdmin(
	ctx context.Context,
	authorizer facade.Authorizer,
	controllerTag names.ControllerTag,
	modelTag names.ModelTag,
) (bool, error) {
	// superusers have admin for all models.
	err := authorizer.HasPermission(ctx, permission.SuperuserAccess, controllerTag)
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return false, err
	}

	if err == nil {
		return true, nil
	}

	err = authorizer.HasPermission(ctx, permission.AdminAccess, modelTag)
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return false, err
	}
	return err == nil, nil
}
