// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"time"

	"github.com/juju/juju/version"
)

// ModelConfigResults contains the result of client API calls
// to get model config values.
type ModelConfigResults struct {
	Config map[string]interface{}
}

// ModelSet contains the arguments for ModelSet client API
// call.
type ModelSet struct {
	Config map[string]interface{}
}

// ModelUnset contains the arguments for ModelUnset client API
// call.
type ModelUnset struct {
	Keys []string
}

// ModifyModelUsers holds the parameters for making Client ShareModel calls.
type ModifyModelUsers struct {
	Changes []ModifyModelUser
}

// ModelAction is an action that can be preformed on a model.
type ModelAction string

// Actions that can be preformed on a model.
const (
	AddModelUser    ModelAction = "add"
	RemoveModelUser ModelAction = "remove"
)

// ModifyModelUser stores the parameters used for a Client.ShareModel call.
type ModifyModelUser struct {
	UserTag string      `json:"user-tag"`
	Action  ModelAction `json:"action"`
}

// SetModelAgentVersion contains the arguments for
// SetModelAgentVersion client API call.
type SetModelAgentVersion struct {
	Version version.Number
}

// ModelUserInfo holds information on a user.
type ModelUserInfo struct {
	UserName       string     `json:"user"`
	DisplayName    string     `json:"displayname"`
	CreatedBy      string     `json:"createdby"`
	DateCreated    time.Time  `json:"datecreated"`
	LastConnection *time.Time `json:"lastconnection"`
}

// ModelUserInfoResult holds the result of an ModelUserInfo call.
type ModelUserInfoResult struct {
	Result *ModelUserInfo `json:"result,omitempty"`
	Error  *Error         `json:"error,omitempty"`
}

// ModelUserInfoResults holds the result of a bulk ModelUserInfo API call.
type ModelUserInfoResults struct {
	Results []ModelUserInfoResult `json:"results"`
}
