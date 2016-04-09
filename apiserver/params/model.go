// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"time"

	"github.com/juju/version"
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

// SetModelAgentVersion contains the arguments for
// SetModelAgentVersion client API call.
type SetModelAgentVersion struct {
	Version version.Number
}

// ModelInfo holds information about the Juju model.
type ModelInfo struct {
	// The json names for the fields below are as per the older
	// field names for backward compatability.
	Name           string `json:"Name"`
	UUID           string `json:"UUID"`
	ControllerUUID string `json:"ServerUUID"`
	ProviderType   string `json:"ProviderType"`
	DefaultSeries  string `json:"DefaultSeries"`

	// OwnerTag is the tag of the user that owns the model.
	OwnerTag string `json:"owner-tag"`

	// Users contains information about the users that have access
	// to the model. Owners and administrators can see all users
	// that have access; other users can only see their own details.
	Users []ModelUserInfo
}

// ModelInfoResult holds the result of a ModelInfo call.
type ModelInfoResult struct {
	Result *ModelInfo `json:"result,omitempty"`
	Error  *Error     `json:"error,omitempty"`
}

// ModelInfoResult holds the result of a bulk ModelInfo call.
type ModelInfoResults struct {
	Results []ModelInfoResult `json:"results"`
}

// ModelUserInfo holds information on a user who has access to a
// model. Owners of a model can see this information for all users
// who have access, so it should not include sensitive information.
type ModelUserInfo struct {
	UserName       string                `json:"user"`
	DisplayName    string                `json:"displayname"`
	LastConnection *time.Time            `json:"lastconnection"`
	Access         ModelAccessPermission `json:"access"`
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

// ModifyModelAccessRequest holds the parameters for making grant and revoke model calls.
type ModifyModelAccessRequest struct {
	Changes []ModifyModelAccess `json:"changes"`
}

type ModifyModelAccess struct {
	UserTag  string                `json:"user-tag"`
	Action   ModelAction           `json:"action"`
	Access   ModelAccessPermission `json:"access"`
	ModelTag string                `json:"model-tag"`
}

// ModelAction is an action that can be performed on a model.
type ModelAction string

// Actions that can be preformed on a model.
const (
	GrantModelAccess  ModelAction = "grant"
	RevokeModelAccess ModelAction = "revoke"
)

// ModelAccessPermission is the type of permission that a user has to access a
// model.
type ModelAccessPermission string

// Model access permissions that may be set on a user.
const (
	ModelReadAccess  ModelAccessPermission = "read"
	ModelWriteAccess ModelAccessPermission = "write"
)
