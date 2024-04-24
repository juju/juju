// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"
	"time"

	"github.com/juju/errors"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// accessService provides the methods from the needed from the access service.
type accessService interface {
	// LastModelConnection gets the time the specified user last connected to the
	// model.
	LastModelConnection(context.Context, coremodel.UUID, string) (time.Time, error)
}

// ModelUserInfo converts permission.UserAccess to params.ModelUserInfo.
func ModelUserInfo(ctx context.Context, st accessService, modelUUID string, user permission.UserAccess) (params.ModelUserInfo, error) {
	access, err := StateToParamsUserAccessPermission(user.Access)
	if err != nil {
		return params.ModelUserInfo{}, errors.Trace(err)
	}

	userLastConn, err := st.LastModelConnection(ctx, coremodel.UUID(modelUUID), user.UserName)
	if err != nil && !state.IsNeverConnectedError(err) {
		return params.ModelUserInfo{}, errors.Trace(err)
	}
	var lastConn *time.Time
	if err == nil {
		lastConn = &userLastConn
	}

	userInfo := params.ModelUserInfo{
		UserName:       user.UserName,
		DisplayName:    user.DisplayName,
		LastConnection: lastConn,
		Access:         access,
	}
	return userInfo, nil
}

// StateToParamsUserAccessPermission converts permission.Access to params.AccessPermission.
func StateToParamsUserAccessPermission(descriptionAccess permission.Access) (params.UserAccessPermission, error) {
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
