// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/state"
)

type modelConnectionAbleBackend interface {
	LastModelConnection(names.UserTag) (time.Time, error)
}

// ModelUserInfo converts permission.UserAccess to params.ModelUserInfo.
func ModelUserInfo(user permission.UserAccess, st modelConnectionAbleBackend) (params.ModelUserInfo, error) {
	access, err := StateToParamsUserAccessPermission(user.Access)
	if err != nil {
		return params.ModelUserInfo{}, errors.Trace(err)
	}

	userLastConn, err := st.LastModelConnection(user.UserTag)
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
