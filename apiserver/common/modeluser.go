// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"time"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

// ModelUser defines the subset of the state.ModelUser type
// that we require to convert to a params.ModelUserInfo.
type ModelUser interface {
	DisplayName() string
	LastConnection() (time.Time, error)
	UserName() string
	UserTag() names.UserTag
	IsReadOnly() bool
	IsReadWrite() bool
	IsAdmin() bool
}

// ModelUserInfo converts *state.ModelUser to params.ModelUserInfo.
func ModelUserInfo(user ModelUser) (params.ModelUserInfo, error) {
	var lastConn *time.Time
	userLastConn, err := user.LastConnection()
	if err == nil {
		lastConn = &userLastConn
	} else if !state.IsNeverConnectedError(err) {
		return params.ModelUserInfo{}, errors.Trace(err)
	}

	access := params.ModelReadAccess
	switch {
	case user.IsAdmin():
		access = params.ModelAdminAccess
	case user.IsReadWrite():
		access = params.ModelWriteAccess
	}

	userInfo := params.ModelUserInfo{
		UserName:       user.UserName(),
		DisplayName:    user.DisplayName(),
		LastConnection: lastConn,
		Access:         access,
	}
	return userInfo, nil
}

// StateToParamsModelAccess converts state.Access to params.AccessPermission.
func StateToParamsModelAccess(stateAccess state.Access) (params.ModelAccessPermission, error) {
	switch stateAccess {
	case state.ReadAccess:
		return params.ModelReadAccess, nil
	case state.WriteAccess:
		return params.ModelWriteAccess, nil
	case state.AdminAccess:
		return params.ModelAdminAccess, nil
	}
	return "", errors.Errorf("invalid model access permission %q", stateAccess)
}
