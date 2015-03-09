// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"time"

	"github.com/juju/juju/version"
)

// EnvironmentConfigResults contains the result of client API calls
// to get environment config values.
type EnvironmentConfigResults struct {
	Config map[string]interface{}
}

// EnvironmentSet contains the arguments for EnvironmentSet client API
// call.
type EnvironmentSet struct {
	Config map[string]interface{}
}

// EnvironmentUnset contains the arguments for EnvironmentUnset client API
// call.
type EnvironmentUnset struct {
	Keys []string
}

// ModifyEnvironUsers holds the parameters for making Client ShareEnvironment calls.
type ModifyEnvironUsers struct {
	Changes []ModifyEnvironUser
}

// EnvironAction is an action that can be preformed on an environment.
type EnvironAction string

// Actions that can be preformed on an environment.
const (
	AddEnvUser    EnvironAction = "add"
	RemoveEnvUser EnvironAction = "remove"
)

// ModifyEnvironUser stores the parameters used for a Client.ShareEnvironment call.
type ModifyEnvironUser struct {
	UserTag string        `json:"user-tag"`
	Action  EnvironAction `json:"action"`
}

// SetEnvironAgentVersion contains the arguments for
// SetEnvironAgentVersion client API call.
type SetEnvironAgentVersion struct {
	Version version.Number
}

// EnvUserInfo holds information on a user.
type EnvUserInfo struct {
	UserName       string     `json:"user"`
	DisplayName    string     `json:"displayname"`
	CreatedBy      string     `json:"createdby"`
	DateCreated    time.Time  `json:"datecreated"`
	LastConnection *time.Time `json:"lastconnection"`
}

// EnvUserInfoResult holds the result of an EnvUserInfo call.
type EnvUserInfoResult struct {
	Result *EnvUserInfo `json:"result,omitempty"`
	Error  *Error       `json:"error,omitempty"`
}

// EnvUserInfoResults holds the result of a bulk EnvUserInfo API call.
type EnvUserInfoResults struct {
	Results []EnvUserInfoResult `json:"results"`
}
