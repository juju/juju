// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/rpc/params"
)

type modelService interface {
	// GetModelUsers will retrieve basic information about users with
	// permissions on the given model UUID.
	GetModelUsers(ctx context.Context, modelUUID coremodel.UUID) ([]coremodel.ModelUserInfo, error)
	// GetModelUser retrieves basic information about the specified
	// model user. A user with no local permission row is still returned,
	// with an empty access level.
	GetModelUser(ctx context.Context, modelUUID coremodel.UUID, name user.Name) (coremodel.ModelUserInfo, error)
}

// ModelUserInfo gets model user info from the modelService and converts it
// into params.ModelUserInfo. If listAllUsers is false, only the caller's
// entry is returned, reporting callerAccess.
func ModelUserInfo(ctx context.Context, service modelService, modelTag names.ModelTag, apiUser user.Name, listAllUsers bool, callerAccess permission.Access) ([]params.ModelUserInfo, error) {
	var userInfo []coremodel.ModelUserInfo
	var err error
	if listAllUsers {
		userInfo, err = service.GetModelUsers(ctx, coremodel.UUID(modelTag.Id()))
	} else {
		var ui coremodel.ModelUserInfo
		ui, err = service.GetModelUser(ctx, coremodel.UUID(modelTag.Id()), apiUser)
		if err == nil {
			ui.Access = callerAccess
		}
		userInfo = append(userInfo, ui)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}

	modelUserInfo := make([]params.ModelUserInfo, len(userInfo))
	for i, mi := range userInfo {
		var lastModelLogin *time.Time
		if !mi.LastModelLogin.IsZero() {
			lmi := mi.LastModelLogin
			lastModelLogin = &lmi
		}
		accessType, err := EncodeAccess(mi.Access)
		if err != nil {
			return nil, errors.Trace(err)
		}
		modelUserInfo[i] = params.ModelUserInfo{
			UserName:       mi.Name.Name(),
			DisplayName:    mi.DisplayName,
			LastConnection: lastModelLogin,
			Access:         accessType,
		}
	}
	return modelUserInfo, nil
}

// EncodeAccess converts permission.Access to params.AccessPermission.
func EncodeAccess(descriptionAccess permission.Access) (params.UserAccessPermission, error) {
	switch descriptionAccess {
	case permission.ReadAccess:
		return params.ModelReadAccess, nil
	case permission.WriteAccess:
		return params.ModelWriteAccess, nil
	case permission.AdminAccess:
		return params.ModelAdminAccess, nil
	}

	return "", errors.NotValidf("model access permission %q", descriptionAccess)
}
