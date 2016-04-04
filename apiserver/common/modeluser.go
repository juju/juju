// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

// ModelUser defines the subset of the state.ModelUser type
// that we require to convert to a params.ModelUserInfo.
type ModelUser interface {
	Access() state.ModelAccess
	DisplayName() string
	LastConnection() (time.Time, error)
	UserName() string
	UserTag() names.UserTag
}

// ModelUserInfo converts *state.ModelUser to params.ModelUserInfo.
func ModelUserInfo(user ModelUser) (params.ModelUserInfo, error) {
	access, err := StateToParamsModelAccess(user.Access())
	if err != nil {
		return params.ModelUserInfo{}, errors.Trace(err)
	}

	var lastConn *time.Time
	userLastConn, err := user.LastConnection()
	if err == nil {
		lastConn = &userLastConn
	} else if !state.IsNeverConnectedError(err) {
		return params.ModelUserInfo{}, errors.Trace(err)
	}

	userInfo := params.ModelUserInfo{
		UserName:       user.UserName(),
		DisplayName:    user.DisplayName(),
		LastConnection: lastConn,
		Access:         access,
	}
	return userInfo, nil
}

// StateToParamsModelAccess converts state.ModelAccess to params.ModelAccessPermission.
func StateToParamsModelAccess(stateAccess state.ModelAccess) (params.ModelAccessPermission, error) {
	switch stateAccess {
	case state.ModelReadAccess:
		return params.ModelReadAccess, nil
	case state.ModelAdminAccess:
		return params.ModelWriteAccess, nil
	}
	return "", errors.Errorf("invalid model access permission %q", stateAccess)
}
