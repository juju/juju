// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/access"
	"github.com/juju/juju/rpc/params"
)

type accessService interface {
	// GetModelUsers will retrieve basic information about all users with
	// permissions on the given model UUID.
	// If the model cannot be found it will return modelerrors.NotFound.
	// If no permissions can be found on the model it will return
	// accesserrors.PermissionNotValid.
	GetModelUsers(ctx context.Context, apiUser user.Name, modelUUID coremodel.UUID) ([]access.ModelUserInfo, error)
}

// ModelUserInfo gets model user info from the accessService and converts it
// into params.ModelUserInfo.
func ModelUserInfo(ctx context.Context, service accessService, apiUser names.UserTag, modelTag names.ModelTag) ([]params.ModelUserInfo, error) {
	userInfo, err := service.GetModelUsers(ctx, user.NameFromTag(apiUser), coremodel.UUID(modelTag.Id()))
	if err != nil {
		return nil, err
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
